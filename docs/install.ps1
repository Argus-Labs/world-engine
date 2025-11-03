#!/usr/bin/env pwsh
# Copyright 2018 the Deno authors. All rights reserved. MIT license.
# TODO(everyone): Keep this script simple and easily auditable.

$ErrorActionPreference = 'Stop'

$Version = if ($v) {
  $v
} elseif ($args.Length -eq 1) {
  $args.Get(0)
} else {
  "latest"
}

$ReleaseUrl = if ($Version -eq "latest") {
  "https://api.github.com/repos/Argus-Labs/world-cli/releases/latest"
} else {
  "https://api.github.com/repos/Argus-Labs/world-cli/releases/tags/$Version"
}

# GitHub require TLS 1.2
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

try {
  $Response = Invoke-WebRequest $ReleaseUrl -UseBasicParsing
  $Release = $Response.Content | ConvertFrom-Json
  $Asset = $Release.assets | Where-Object { $_.name -like "*Windows_x86_64.zip" }

  if (!$Asset) {
    Write-Error "No binary found for Windows x86_64 version $Version - see github.com/Argus-Labs/world-cli/releases for all versions"
    Exit 1
  }

  $DownloadUrl = $Asset.browser_download_url
}
catch {
  $StatusCode = $_.Exception.Response.StatusCode.value__
  if ($StatusCode -eq 404) {
    Write-Error "Unable to find release version $Version - see github.com/Argus-Labs/world-cli/releases for all versions"
  } else {
    $Request = $_.Exception
    Write-Error "Error while fetching releases: $Request"
  }
  Exit 1
}

$WorldInstall = $env:WORLD_INSTALL
$BinDir = if ($WorldInstall) {
  "$WorldInstall\bin"
} else {
  "$Home\.worldcli\bin"
}

$WorldZip = "$BinDir\world.zip"
$WorldExe = "$BinDir\world.exe"

if (!(Test-Path $BinDir)) {
  New-Item $BinDir -ItemType Directory | Out-Null
}

$prevProgressPreference = $ProgressPreference
try {
  # Invoke-WebRequest on older powershell versions has severe transfer
  # performance issues due to progress bar rendering - the screen updates
  # end up throttling the download itself. Disable progress on these older
  # versions.
  if ($PSVersionTable.PSVersion.Major -lt 7) {
    Write-Output "Downloading world..."
    $ProgressPreference = "SilentlyContinue"
  }

  Invoke-WebRequest $DownloadUrl -OutFile $WorldZip -UseBasicParsing
} finally {
  $ProgressPreference = $prevProgressPreference
}

if (Get-Command Expand-Archive -ErrorAction SilentlyContinue) {
  Expand-Archive $WorldZip -Destination $BinDir -Force
} else {
  Remove-Item $WorldExe -ErrorAction SilentlyContinue
  Add-Type -AssemblyName System.IO.Compression.FileSystem
  [IO.Compression.ZipFile]::ExtractToDirectory($WorldZip, $BinDir)
}

Remove-Item $WorldZip

$User = [EnvironmentVariableTarget]::User
$Path = [Environment]::GetEnvironmentVariable('Path', $User)
if (!(";$Path;".ToLower() -like "*;$BinDir;*".ToLower())) {
  [Environment]::SetEnvironmentVariable('Path', "$Path;$BinDir", $User)
  $Env:Path += ";$BinDir"
}

Write-Output "world was installed successfully to $WorldExe"
Write-Output "Run 'world --help' to get started"
