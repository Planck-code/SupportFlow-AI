# Shared setup for the MVP and AI demos. Dot-source after defining BaseUrl and AdminCredential.
Add-Type -AssemblyName System.Net.Http
if (-not $BaseUrl.IsAbsoluteUri -or $BaseUrl.Scheme -notin 'http','https' -or $BaseUrl.UserInfo -or $BaseUrl.Query -or $BaseUrl.Fragment) {
    throw 'BaseUrl must be an absolute HTTP(S) API URL without credentials, query or fragment.'
}
if (-not $AdminCredential) {
    $email = Read-Host 'Administrator email'
    $AdminCredential = [PSCredential]::new($email, (Read-Host 'Administrator password' -AsSecureString))
}

function Confirm-Step([bool]$Condition, [string]$Step) {
    if (-not $Condition) { throw "Demo verification failed: $Step" }
}

function Invoke-Api($Method, $Path, $Expected, $Token, $Body) {
    $address = $BaseUrl.AbsoluteUri.TrimEnd('/') + $Path
    $request = [Net.Http.HttpRequestMessage]::new([Net.Http.HttpMethod]::new($Method), $address)
    $response = $null
    try {
        if ($Token) { $request.Headers.Authorization = [Net.Http.Headers.AuthenticationHeaderValue]::new('Bearer', $Token) }
        if ($null -ne $Body) {
            $json = ConvertTo-Json -Compress -InputObject $Body
            $request.Content = [Net.Http.StringContent]::new($json, [Text.Encoding]::UTF8, 'application/json')
        }
        try { $response = $client.SendAsync($request).GetAwaiter().GetResult() }
        catch { throw "Network request failed: $Method $Path" }
        $status = [int]$response.StatusCode
        if ($status -ne $Expected) { throw "$Method $Path returned HTTP $status (expected $Expected)." }
        $bodyText = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
        try { $result = ConvertFrom-Json -InputObject $bodyText }
        catch { throw "$Method $Path returned invalid JSON." }
        if ($Expected -eq 404) {
            Confirm-Step ($result.code -eq 'not_found' -and $result.request_id) "hidden ticket: $Path"
        }
        return $result
    } finally {
        if ($response) { $response.Dispose() }
        $request.Dispose()
    }
}

function Connect-Account($Email, $Password, $Role) {
    $login = Invoke-Api POST '/auth/login' 200 $null @{email=$Email; password=$Password}
    Confirm-Step ($login.token_type -eq 'Bearer' -and $login.access_token -and $login.expires_in -gt 0) 'login'
    $me = Invoke-Api GET '/users/me' 200 $login.access_token $null
    Confirm-Step ($me.role -eq $Role -and $me.email -eq $Email -and $me.id -gt 0) 'authenticated identity'
    return $login.access_token
}

function New-DemoPassword {
    $bytes = New-Object byte[] 32
    $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($bytes); return [Convert]::ToBase64String($bytes) }
    finally { $rng.Dispose() }
}
