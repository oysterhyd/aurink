$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Windows.Forms
$word = New-Object -ComObject Word.Application
$word.Visible = $false
$word.DisplayAlerts = 0
try {
    $version = $word.Version
    foreach ($case in 1..5) {
        $doc = $word.Documents.Add()
        try {
            $s = $word.Selection
            switch ($case) {
                1 {
                    $s.TypeText('A plain sentence, then ')
                    $s.Font.Bold = 1
                    $s.TypeText('bold words')
                    $s.Font.Bold = 0
                    $s.TypeText(' and ')
                    $s.Font.Italic = 1
                    $s.TypeText('italic text')
                    $s.Font.Italic = 0
                    $s.TypeText('.')
                }
                2 {
                    $s.Style = -2
                    $s.TypeText('Project heading')
                    $s.TypeParagraph()
                    $s.Style = -1
                    $s.TypeText('A second paragraph with punctuation & symbols.')
                }
                3 {
                    $s.Range.ListFormat.ApplyBulletDefault()
                    $s.TypeText('First item')
                    $s.TypeParagraph()
                    $s.TypeText('Second item')
                    $s.TypeParagraph()
                    $s.TypeText('Third item')
                }
                4 {
                    $table = $doc.Tables.Add($s.Range, 3, 2)
                    $table.Cell(1,1).Range.Text = 'Name'
                    $table.Cell(1,2).Range.Text = 'Count'
                    $table.Cell(2,1).Range.Text = 'Alpha'
                    $table.Cell(2,2).Range.Text = '12'
                    $table.Cell(3,1).Range.Text = 'Beta'
                    $table.Cell(3,2).Range.Text = '7'
                }
                5 {
                    $s.TypeText('For details visit ')
                    $doc.Hyperlinks.Add($s.Range, 'https://example.org/reference', $null, $null, 'the reference') | Out-Null
                    $s.EndKey(6) | Out-Null
                    $s.TypeParagraph()
                    $s.TypeText('Accents: café and naïve.')
                }
            }
            $doc.Content.Copy()
            $data = [System.Windows.Forms.Clipboard]::GetData('HTML Format')
            if (-not $data -or $data -notmatch '(?s)<!--StartFragment-->(.*?)<!--EndFragment-->') {
                throw 'Word did not expose an HTML clipboard fragment.'
            }
            $file = Join-Path $PSScriptRoot ('word-{0:D2}.html' -f $case)
            [System.IO.File]::WriteAllText($file, $Matches[1], [System.Text.UTF8Encoding]::new($false))
        } finally {
            $doc.Close(0)
        }
    }
    "Word $version generated five HTML clipboard fragments."
} finally {
    $word.Quit()
}
