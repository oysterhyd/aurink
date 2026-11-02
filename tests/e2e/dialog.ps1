# dialog.ps1 answers a native dialog of a process through UI Automation, for
# tests/e2e (Windows only). It waits for the process's dialog window, prints
# its title and buttons, then either types a path into a file dialog and
# confirms it (-Path) or presses the button named -Button.
param(
	[Parameter(Mandatory)][int]$ProcId,
	[string]$Path,
	[string]$Button,
	[int]$Timeout = 20
)
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
$A = [System.Windows.Automation.AutomationElement]
$Scope = [System.Windows.Automation.TreeScope]
$P = [System.Windows.Automation.AutomationElement]

$cond = New-Object System.Windows.Automation.AndCondition(
	(New-Object System.Windows.Automation.PropertyCondition($P::ProcessIdProperty, $ProcId)),
	(New-Object System.Windows.Automation.PropertyCondition($P::ClassNameProperty, '#32770')))
$deadline = (Get-Date).AddSeconds($Timeout)
$dlg = $null
while (-not $dlg) {
	# A dialog shows under the window that owns it, or at the top.
	$dlg = $A::RootElement.FindFirst($Scope::Children, $cond)
	if (-not $dlg) {
		$mine = New-Object System.Windows.Automation.PropertyCondition($P::ProcessIdProperty, $ProcId)
		foreach ($w in $A::RootElement.FindAll($Scope::Children, $mine)) {
			$dlg = $w.FindFirst($Scope::Children, $cond)
			if ($dlg) { break }
		}
	}
	if (-not $dlg) {
		if ((Get-Date) -gt $deadline) {
			$A::RootElement.FindAll($Scope::Children, [System.Windows.Automation.Condition]::TrueCondition) |
				ForEach-Object { "window: pid " + $_.Current.ProcessId + " class " + $_.Current.ClassName + " name " + $_.Current.Name }
			throw "no dialog of process $ProcId"
		}
		Start-Sleep -Milliseconds 150
	}
}
# Buttons: UI Automation buttons, or the task dialog's push buttons (panes).
$isButton = New-Object System.Windows.Automation.OrCondition(
	(New-Object System.Windows.Automation.PropertyCondition($P::ControlTypeProperty, [System.Windows.Automation.ControlType]::Button)),
	(New-Object System.Windows.Automation.PropertyCondition($P::ClassNameProperty, 'CCPushButton')))
$buttons = $dlg.FindAll($Scope::Descendants, $isButton) | ForEach-Object { $_.Current.Name }
"title: " + $dlg.Current.Name
"buttons: " + ($buttons -join ' | ')

Add-Type -Namespace W -Name U -MemberDefinition @'
[DllImport("user32.dll", CharSet = CharSet.Unicode)]
public static extern IntPtr SendMessage(IntPtr h, int msg, IntPtr w, string l);
[DllImport("user32.dll")]
public static extern IntPtr SendMessage(IntPtr h, int msg, IntPtr w, IntPtr l);
'@

# Press invokes an element, or clicks a plain Win32 button by its handle.
function Press($el) {
	$inv = $null
	if ($el.TryGetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern, [ref]$inv)) {
		$inv.Invoke()
	} else {
		[void][W.U]::SendMessage([IntPtr]$el.Current.NativeWindowHandle, 0x00F5, [IntPtr]::Zero, [IntPtr]::Zero) # BM_CLICK
	}
}

if ($Path) {
	# The file name box: the edit 1148 (Open) or 1001 (Save), then the default
	# button (1).
	$byId = New-Object System.Windows.Automation.OrCondition(
		(New-Object System.Windows.Automation.PropertyCondition($P::AutomationIdProperty, '1148')),
		(New-Object System.Windows.Automation.PropertyCondition($P::AutomationIdProperty, '1001')))
	$box = $null
	while (-not $box) {
		# The dialog fills in its controls after it shows; the box is a plain
		# Win32 edit, set through its handle.
		foreach ($e in $dlg.FindAll($Scope::Descendants, $byId)) {
			if ($e.Current.ClassName -eq 'Edit') { $box = $e; break }
		}
		if (-not $box) {
			if ((Get-Date) -gt $deadline) {
				$dlg.FindAll($Scope::Descendants, [System.Windows.Automation.Condition]::TrueCondition) |
					ForEach-Object { "  " + $_.Current.ControlType.ProgrammaticName + " [" + $_.Current.AutomationId + "] " + $_.Current.Name + " (" + $_.Current.ClassName + ")" }
				throw 'no file name box'
			}
			Start-Sleep -Milliseconds 150
		}
	}
	"default name: " + $box.Current.Name
	[void][W.U]::SendMessage([IntPtr]$box.Current.NativeWindowHandle, 0x000C, [IntPtr]::Zero, $Path) # WM_SETTEXT
	# OK (the dialog's default button, IDOK) as Enter would press it.
	[void][W.U]::SendMessage([IntPtr]$dlg.Current.NativeWindowHandle, 0x0111, [IntPtr]1, [IntPtr]::Zero) # WM_COMMAND
} elseif ($Button) {
	$named = New-Object System.Windows.Automation.AndCondition($isButton,
		(New-Object System.Windows.Automation.PropertyCondition($P::NameProperty, $Button)))
	$b = $dlg.FindFirst($Scope::Descendants, $named)
	while (-not $b -and (Get-Date) -lt $deadline) {
		Start-Sleep -Milliseconds 150
		$b = $dlg.FindFirst($Scope::Descendants, $named)
	}
	if (-not $b) {
		$dlg.FindAll($Scope::Descendants, [System.Windows.Automation.Condition]::TrueCondition) |
			ForEach-Object { "  " + $_.Current.ControlType.ProgrammaticName + " [" + $_.Current.AutomationId + "] " + $_.Current.Name + " (" + $_.Current.ClassName + ")" }
		throw "no button $Button"
	}
	Press $b
}
