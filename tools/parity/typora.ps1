# Runs tools/parity/scenarios.json in Typora (Windows) and writes what each
# scenario leaves in the file, as JSON {name: source}. Each scenario opens its
# own file in a new Typora window, so other Typora windows are not touched.
# Usage: pwsh -File tools/parity/typora.ps1 -Out typora.json [-Typora C:\path\Typora.exe] [-Only name]
param([string]$Out, [string]$Typora = "C:\typora\Typora\Typora.exe", [string]$Only = "", [string]$Work = "")
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Windows.Forms
Add-Type @"
using System; using System.Text; using System.Runtime.InteropServices;
public class TP {
  public delegate bool EnumProc(IntPtr h, IntPtr l);
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumProc f, IntPtr l);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern int GetWindowText(IntPtr h, StringBuilder s, int n);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
  [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
  public static IntPtr Find(string part) {
    IntPtr found = IntPtr.Zero;
    EnumWindows((h, l) => { if (!IsWindowVisible(h)) return true; var sb = new StringBuilder(512); GetWindowText(h, sb, 512);
      if (sb.ToString().Contains(part)) { found = h; return false; } return true; }, IntPtr.Zero);
    return found;
  }
}
"@
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$scenarios = Get-Content (Join-Path $root "scenarios.json") -Raw | ConvertFrom-Json
if (-not $Work) { $Work = Join-Path ([IO.Path]::GetTempPath()) "aurink-parity" }
New-Item -ItemType Directory -Force $Work | Out-Null

$named = @{ ENTER = "{ENTER}"; BS = "{BS}"; DEL = "{DEL}"; TAB = "{TAB}"; HOME = "{HOME}"; END = "{END}";
  UP = "{UP}"; DOWN = "{DOWN}"; LEFT = "{LEFT}"; RIGHT = "{RIGHT}"; ESC = "{ESC}" }
function Esc([string]$c) { if ("+^%~(){}[]".Contains($c)) { return "{$c}" } return $c }
# keys: text with {MODS+KEY} tokens -> a list of SendKeys strings, one per token or character.
function Steps([string]$keys) {
  $out = @()
  $i = 0
  while ($i -lt $keys.Length) {
    $c = $keys[$i]
    if ($c -eq '{') {
      $j = $keys.IndexOf('}', $i + 2)
      $tok = $keys.Substring($i + 1, $j - $i - 1)
      $i = $j + 1
      $mods = ""; $key = $tok
      if ($tok -match '^([CSA]+)\+(.+)$') {
        $key = $Matches[2]
        foreach ($m in $Matches[1].ToCharArray()) { $mods += @{ 'C' = '^'; 'S' = '+'; 'A' = '%' }[[string]$m] }
      }
      if ($named.ContainsKey($key)) { $out += $mods + $named[$key] } else { $out += $mods + (Esc $key) }
    } else {
      $out += (Esc ([string]$c)); $i++
    }
  }
  return $out
}

$results = [ordered]@{}
foreach ($s in $scenarios) {
  if ($Only -and -not (($Only -split ",") -contains $s.name)) { continue }
  $f = Join-Path $Work ("par_" + $s.name + ".md")
  [IO.File]::WriteAllText($f, $s.doc)
  Start-Process $Typora -ArgumentList "`"$f`""
  $tag = "par_" + $s.name + ".md"
  $h = [IntPtr]::Zero
  for ($k = 0; $k -lt 60 -and $h -eq [IntPtr]::Zero; $k++) { Start-Sleep -Milliseconds 250; $h = [TP]::Find($tag) }
  if ($h -eq [IntPtr]::Zero) { Write-Host "no window for $($s.name)"; continue }
  Start-Sleep -Milliseconds 1200
  $ok = $true
  foreach ($step in (Steps $s.keys)) {
    [TP]::SetForegroundWindow($h) | Out-Null
    Start-Sleep -Milliseconds 80
    if ([TP]::GetForegroundWindow() -ne $h) { Write-Host "lost focus in $($s.name)"; $ok = $false; break }
    [System.Windows.Forms.SendKeys]::SendWait($step)
    Start-Sleep -Milliseconds 250
  }
  [TP]::SetForegroundWindow($h) | Out-Null
  Start-Sleep -Milliseconds 100
  if ([TP]::GetForegroundWindow() -eq $h) {
    [System.Windows.Forms.SendKeys]::SendWait("^s"); Start-Sleep -Milliseconds 500
    [System.Windows.Forms.SendKeys]::SendWait("^w"); Start-Sleep -Milliseconds 500
  }
  if ($ok) { $results[$s.name] = [IO.File]::ReadAllText($f).TrimStart([char]0xFEFF) }
  Write-Host "$($s.name) done"
}
$results | ConvertTo-Json | Set-Content -Path $Out -Encoding utf8
