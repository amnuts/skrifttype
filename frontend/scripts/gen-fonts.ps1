# Generates src/mock/fonts.generated.json from the machine's installed fonts.
# Re-runnable. Output shape matches FontRecord[] in src/data/types.ts.
#
# Mock-demo choices:
#  - Per-user (HKCU) fonts are treated as toggleable `library` fonts; machine
#    (HKLM) fonts are read-only `system` fonts. This gives a realistic mix so the
#    activate/deactivate interaction is meaningful.
#  - Families are grouped across space- AND hyphen-separated (PostScript) names,
#    e.g. "ACaslonPro-Bold" + "ACaslonPro-Italic" -> one "ACaslonPro" family.
$ErrorActionPreference = 'Stop'
$outFile = Join-Path $PSScriptRoot '..\src\mock\fonts.generated.json'

function Read-FontKey($root, $base, $source) {
  $items = @()
  $key = Get-ItemProperty $root -ErrorAction SilentlyContinue
  if (-not $key) { return $items }
  foreach ($p in $key.PSObject.Properties) {
    if ($p.Name -like 'PS*') { continue }
    $file = [string]$p.Value
    $path = if ([System.IO.Path]::IsPathRooted($file)) { $file } else { Join-Path $base $file }
    $items += [pscustomobject]@{ display = $p.Name; path = $path; source = $source }
  }
  return $items
}

$winFonts = Join-Path $env:WINDIR 'Fonts'
$userFonts = Join-Path $env:LOCALAPPDATA 'Microsoft\Windows\Fonts'
$entries  = @()
$entries += Read-FontKey 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts' $winFonts 'system-hklm'
$entries += Read-FontKey 'HKCU:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts' $userFonts 'library'

$weightMap = @{ 'thin'=100;'hairline'=100;'extralight'=200;'ultralight'=200;'light'=300;'regular'=400;'normal'=400;'book'=400;'medium'=500;'semibold'=600;'demibold'=600;'demi'=600;'bold'=700;'extrabold'=800;'ultrabold'=800;'black'=900;'heavy'=900 }
$styleTokens = @('italic','oblique','condensed','expanded','narrow','wide','semi','demi','ultra','extra','caption','display','text','subhead','poster')
$monoHints   = @('mono','code','consol','courier','terminal','typewriter')
$scriptHints = @('script','hand','brush','caveat','pacifico','calligraph','signature','segoe script','vivaldi','lucida handwriting','segoe print')
$displayHints= @('display','poster','headline','impact','stencil','decorative','jokerman','showcard','chiller','ravie')
$serifHints  = @('serif','slab','times','georgia','garamond','roman','playfair','minion','book antiqua','cambria','constantia','caslon','baskerville','bodoni','didot','palatino','sylfaen','sitka','ebrima','rockwell','cabin','centaur','plantin','sabon','utopia','perpetua','goudy','bookman','century')

function Classify($name) {
  $n = $name.ToLower()
  foreach ($h in $monoHints)    { if ($n -like "*$h*") { return 'mono' } }
  foreach ($h in $scriptHints)  { if ($n -like "*$h*") { return 'script' } }
  foreach ($h in $displayHints) { if ($n -like "*$h*") { return 'display' } }
  foreach ($h in $serifHints)   { if ($n -like "*$h*") { return 'serif' } }
  return 'sans'
}

# Insert spaces into camelCase / before digits so style detection works:
# "SemiboldItalic" -> "Semibold Italic", "3270Medium" -> "3270 Medium".
function Expand-Camel($s) {
  $s = [regex]::Replace($s, '(?<=[a-z0-9])(?=[A-Z])', ' ')
  $s = [regex]::Replace($s, '(?<=[A-Za-z])(?=[0-9])', ' ')
  return $s.Trim()
}

# Returns @{ family; style } from a (suffix-stripped) display name.
function Split-FamilyStyle($disp) {
  if ($disp.Contains('-') -and ($disp -notmatch '\s')) {
    # PostScript-style "Family-Style" (no spaces): split on the FIRST hyphen.
    $idx = $disp.IndexOf('-')
    $family = $disp.Substring(0, $idx).Trim()
    $style  = (Expand-Camel $disp.Substring($idx + 1))
    if (-not $style) { $style = 'Regular' }
    return @{ family = $family; style = $style }
  }
  $tokens = ($disp -split '\s+')
  $styleWords = @(); $i = $tokens.Count - 1
  while ($i -ge 1) {
    $t = $tokens[$i].ToLower()
    if ($weightMap.ContainsKey($t) -or ($styleTokens -contains $t)) { $styleWords = ,$tokens[$i] + $styleWords; $i-- } else { break }
  }
  $family = ($tokens[0..$i] -join ' ')
  $style  = if ($styleWords.Count) { $styleWords -join ' ' } else { 'Regular' }
  return @{ family = $family; style = $style }
}

# Synthetic-but-plausible feature/range data per classification (mock approximation).
$featBy = @{
  serif   = @{ gsub=@('liga','dlig','smcp','onum','pnum','ss01'); gpos=@('kern','mark') }
  sans    = @{ gsub=@('liga','tnum','pnum','ss01','ss02','zero'); gpos=@('kern') }
  mono    = @{ gsub=@('liga','calt','zero','ss01','cv01','cv02'); gpos=@('kern') }
  script  = @{ gsub=@('liga','dlig','swsh','calt','salt');        gpos=@('kern','mark','curs') }
  display = @{ gsub=@('liga','case','onum');                       gpos=@('kern') }
}
$rangesCommon = @(@{start=0x0020;end=0x007E}, @{start=0x00A0;end=0x00FF}, @{start=0x0100;end=0x017F}, @{start=0x2018;end=0x2022})

$families = @{}
foreach ($e in $entries) {
  $disp = $e.display -replace '\s*\((TrueType|OpenType|All res)\)\s*$',''
  $ext  = [System.IO.Path]::GetExtension($e.path).TrimStart('.').ToLower()
  if ($ext -notin @('ttf','otf','ttc','otc')) { continue }

  $fs = Split-FamilyStyle $disp
  $family = $fs.family
  $style  = $fs.style
  if (-not $family) { continue }
  $sl = $style.ToLower()
  $italic = $sl -match 'italic|oblique'
  $weight = 400; foreach ($w in $weightMap.Keys) { if ($sl -match "\b$w") { $weight = $weightMap[$w] } }

  # First style of a family wins the source. A family already present as 'library'
  # stays library (per-user installs take precedence for the demo's toggle story).
  if (-not $families.ContainsKey($family)) {
    $cls = Classify $family
    $families[$family] = [pscustomobject]@{
      family=$family; cssFamily=$family; classification=$cls; source=$e.source;
      styles=@(); designer=$null; foundry=$null; version=$null; format=$ext; available=$true;
      parseError=$null; isVariable=$false; gsubFeatures=$featBy[$cls].gsub; gposFeatures=$featBy[$cls].gpos; unicodeRanges=$rangesCommon
    }
  } elseif ($e.source -eq 'library') {
    $families[$family].source = 'library'
  }

  # Dedupe styles by name within a family.
  if (-not ($families[$family].styles | Where-Object { $_.styleName -eq $style })) {
    $families[$family].styles += [pscustomobject]@{ styleName=$style; weight=$weight; italic=[bool]$italic; path=$e.path }
  }
}

$id = 0
$records = $families.Values | Sort-Object family | ForEach-Object {
  $id++
  $_ | Add-Member -NotePropertyName id -NotePropertyValue $id -PassThru |
       Add-Member -NotePropertyName styleCount -NotePropertyValue $_.styles.Count -PassThru
}

$json = $records | ConvertTo-Json -Depth 8
New-Item -ItemType Directory -Force -Path (Split-Path $outFile) | Out-Null
Set-Content -Path $outFile -Value $json -Encoding UTF8
$libCount = @($records | Where-Object { $_.source -eq 'library' }).Count
Write-Host "Wrote $($records.Count) families ($libCount library, $($records.Count - $libCount) system) to $outFile"
