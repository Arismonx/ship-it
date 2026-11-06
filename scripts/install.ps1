# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Grégoire Favreau
#
# Installs ship from a GitHub release of GregoireF/ship-it, on Windows
# (Windows PowerShell 5.1 or PowerShell 7), into a user directory (no admin):
#
#   irm https://raw.githubusercontent.com/GregoireF/ship-it/main/scripts/install.ps1 | iex
#
# It downloads the zip archive for this system and checksums.txt, checks the
# archive's SHA-256 against checksums.txt and stops on any mismatch. It also
# checks where the release comes from, with the first verifier it finds:
# - cosign: checksums.txt was signed by the release workflow of
#   GregoireF/ship-it for that tag (keyless, Sigstore);
# - otherwise the GitHub CLI, logged in: `gh attestation verify` checks the
#   archive's build provenance, made by that same workflow for that tag.
# These are the checks of docs/design/workflows.md §5. Without either, it
# checks the SHA-256 only, unless -RequireSignature is given. Nothing
# downloaded is run or installed before these checks pass.
#
# Settings, as parameters when the script is run as a file
# (.\install.ps1 -Version 0.1.0) or as environment variables (a switch
# variable counts only when it is 1 or true):
#   -Version X.Y.Z      SHIP_VERSION               release to install (default: latest)
#   -Dir DIR            SHIP_INSTALL_DIR           default: %LOCALAPPDATA%\Programs\ship
#   -NoCosign           SHIP_NO_COSIGN=1           do not use cosign even if it is there
#   -RequireSignature   SHIP_REQUIRE_SIGNATURE=1   fail when neither cosign nor gh can check
# For tests and mirrors only: SHIP_INSTALL_BASE_URL replaces
# https://github.com/GregoireF/ship-it/releases/download/<tag> (it needs a
# version; a local folder works too).

param(
    [string]$Version = $env:SHIP_VERSION,
    [string]$Dir = $env:SHIP_INSTALL_DIR,
    [switch]$NoCosign,
    [switch]$RequireSignature
)

# True when $Value is 1 or true. Not [bool]: [bool]'0' and [bool]'false' are
# both True in PowerShell, so SHIP_NO_COSIGN=0 would turn the check off.
function Test-ShipTrue {
    param([string]$Value)
    return ($Value -ceq '1' -or $Value -ceq 'true')
}

# Everything runs in a function: under `irm | iex`, an error stops the
# function without closing the user's window.
function Install-Ship {
    param([string]$Version, [string]$Dir, [bool]$NoCosign, [bool]$RequireSignature, [string]$BaseUrl)

    $ErrorActionPreference = 'Stop'
    # Invoke-WebRequest draws a slow progress bar in Windows PowerShell 5.1.
    $ProgressPreference = 'SilentlyContinue'
    # Windows PowerShell 5.1 may still default to TLS 1.0.
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    $repo = 'GregoireF/ship-it'
    # $IsWindows exists in PowerShell 7 only; 5.1 runs on Windows only.
    if ($PSVersionTable.PSEdition -eq 'Core' -and -not $IsWindows) {
        throw 'ship install: this script is for Windows, use scripts/install.sh on Linux and macOS'
    }

    # The OS architecture, not the process one: an x64 PowerShell on an
    # ARM64 machine still gets the native binary.
    $arch = $null
    try {
        $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
    } catch {
        $arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    }
    switch -Regex ($arch) {
        '^(X64|AMD64)$' { $arch = 'amd64' }
        '^(Arm64|ARM64)$' { $arch = 'arm64' }
        default { throw "ship install: unsupported architecture $arch, see https://github.com/$repo#install" }
    }

    if (-not $Version) {
        if ($BaseUrl) { throw 'ship install: SHIP_INSTALL_BASE_URL needs a version' }
        # The tag of the latest release, read from the redirect of
        # /releases/latest (no API call, so no API rate limit).
        $final = ''
        try {
            $request = [System.Net.HttpWebRequest]::Create("https://github.com/$repo/releases/latest")
            $request.Method = 'HEAD'
            $request.AllowAutoRedirect = $true
            $response = $request.GetResponse()
            try { $final = $response.ResponseUri.AbsoluteUri } finally { $response.Close() }
        } catch {
            throw "ship install: could not find the latest release of ${repo}: $($_.Exception.Message)"
        }
        if ($final -notmatch '/releases/tag/([^/?#]+)$') { throw "ship install: could not find the latest release of $repo" }
        $tag = $Matches[1]
    } else {
        $tag = 'v' + ($Version -replace '^v', '')
    }
    # The tag goes into URLs and file names: semver characters only.
    if ($tag -cnotmatch '^v[0-9][0-9A-Za-z.+-]*$') { throw "ship install: not a version: $tag" }
    $Version = $tag.Substring(1)
    if (-not $BaseUrl) { $BaseUrl = "https://github.com/$repo/releases/download/$tag" }
    if (-not $Dir) { $Dir = Join-Path $env:LOCALAPPDATA 'Programs\ship' }
    $archive = "ship_${Version}_windows_${arch}.zip"

    # Which verifier checks the origin of the release: cosign, else a
    # logged-in gh, else none.
    $cosign = Get-Command cosign -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    $gh = Get-Command gh -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    $verifier = ''
    if (-not $NoCosign -and $cosign) {
        $verifier = 'cosign'
    } elseif ($gh) {
        & $gh.Source auth status *> $null
        if ($LASTEXITCODE -eq 0) { $verifier = 'gh' }
    }
    if (-not $verifier -and $RequireSignature) {
        throw 'ship install: -RequireSignature: install cosign, or the GitHub CLI (gh) and run gh auth login'
    }

    $tmp = Join-Path ([System.IO.Path]::GetTempPath()) ('ship-install-' + [System.Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        # A URL, or a local folder (tests).
        $fetch = {
            param([string]$Name)
            $out = Join-Path $tmp $Name
            if ($BaseUrl -match '^https://') {
                Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl/$Name" -OutFile $out
            } elseif ($BaseUrl -match '^[a-z][a-z0-9+.-]*://') {
                throw "ship install: only https:// or a local folder can be used, not $BaseUrl"
            } else {
                Copy-Item -LiteralPath (Join-Path $BaseUrl $Name) -Destination $out
            }
            $out
        }

        Write-Host "Downloading ship $Version for windows/$arch"
        $archivePath = & $fetch $archive
        $sumsPath = & $fetch 'checksums.txt'

        if ($verifier -eq 'cosign') {
            $bundlePath = & $fetch 'checksums.txt.sigstore.json'
            & $cosign.Source verify-blob $sumsPath `
                --bundle $bundlePath `
                --certificate-identity "https://github.com/$repo/.github/workflows/release.yml@refs/tags/$tag" `
                --certificate-oidc-issuer 'https://token.actions.githubusercontent.com'
            if ($LASTEXITCODE -ne 0) { throw "ship install: checksums.txt is not signed by the release workflow of $repo for $tag" }
            Write-Host 'Signature of checksums.txt verified with cosign'
        }

        # Exactly one line "<sha256>  <archive>" must name the archive.
        $expected = @(Get-Content -LiteralPath $sumsPath | ForEach-Object {
                $f = $_ -split '\s+', 2
                if ($f.Count -eq 2 -and ($f[1] -ceq $archive -or $f[1] -ceq "*$archive")) { $f[0].ToLowerInvariant() }
            })
        if ($expected.Count -eq 0) { throw "ship install: $archive is not listed in checksums.txt" }
        if ($expected.Count -gt 1) { throw "ship install: $archive is listed more than once in checksums.txt" }
        $actual = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($actual -cne $expected[0]) { throw "ship install: SHA-256 mismatch for ${archive}: expected $($expected[0]), got $actual" }
        Write-Host 'SHA-256 verified'

        if ($verifier -eq 'gh') {
            & $gh.Source attestation verify $archivePath -R $repo `
                --signer-workflow "github.com/$repo/.github/workflows/release.yml" `
                --source-ref "refs/tags/$tag" | Out-Null
            if ($LASTEXITCODE -ne 0) { throw "ship install: $archive has no build provenance from the release workflow of $repo for $tag" }
            Write-Host "Build provenance of $archive verified with gh"
        } elseif (-not $verifier) {
            Write-Host 'Neither cosign nor a logged-in gh found: checked the SHA-256 only'
            Write-Host '(install one of them to also check where the release comes from)'
        }

        $x = Join-Path $tmp 'x'
        Expand-Archive -LiteralPath $archivePath -DestinationPath $x
        $exe = Join-Path $x 'ship.exe'
        if (-not (Test-Path -LiteralPath $exe -PathType Leaf)) { throw "ship install: $archive holds no ship.exe" }
        New-Item -ItemType Directory -Force -Path $Dir | Out-Null
        $target = Join-Path $Dir 'ship.exe'
        # A running ship.exe (`ship mcp` under an editor) cannot be
        # overwritten, but it can be renamed: move it aside to ship.exe.old,
        # copy the new one, then delete the old one if nothing runs it.
        $old = $null
        if (Test-Path -LiteralPath $target -PathType Leaf) {
            $old = "$target.old"
            if (Test-Path -LiteralPath $old) {
                Remove-Item -LiteralPath $old -Force -ErrorAction SilentlyContinue
                # Still there: a ship.exe.old from an earlier install is running.
                if (Test-Path -LiteralPath $old) { $old = "$target.$([System.Guid]::NewGuid().ToString('N')).old" }
            }
            Move-Item -LiteralPath $target -Destination $old
        }
        try {
            Copy-Item -LiteralPath $exe -Destination $target
        } catch {
            if ($old) { Move-Item -LiteralPath $old -Destination $target -Force -ErrorAction SilentlyContinue }
            throw
        }
        if ($old) {
            Remove-Item -LiteralPath $old -Force -ErrorAction SilentlyContinue
            if (Test-Path -LiteralPath $old) {
                Write-Host "The previous ship.exe is still running; delete $old once it stops."
            }
        }
        Write-Host "Installed $target"
    } finally {
        Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }

    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $onPath = @(($userPath, $env:Path) -join ';' -split ';' | Where-Object { $_.TrimEnd('\') -ieq $Dir.TrimEnd('\') }).Count -gt 0
    if ($onPath) {
        Write-Host 'Run: ship doctor'
    } else {
        Write-Host ''
        Write-Host "$Dir is not on your PATH. To add it for your user, run this once,"
        Write-Host 'then open a new terminal:'
        Write-Host ''
        Write-Host "  [Environment]::SetEnvironmentVariable('Path', [Environment]::GetEnvironmentVariable('Path', 'User') + ';$Dir', 'User')"
        Write-Host ''
        Write-Host 'Then run: ship doctor'
    }
}

Install-Ship -Version $Version -Dir $Dir `
    -NoCosign ($NoCosign.IsPresent -or (Test-ShipTrue $env:SHIP_NO_COSIGN)) `
    -RequireSignature ($RequireSignature.IsPresent -or (Test-ShipTrue $env:SHIP_REQUIRE_SIGNATURE)) `
    -BaseUrl $env:SHIP_INSTALL_BASE_URL
