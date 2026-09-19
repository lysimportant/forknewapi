#Requires -Version 7.0

<#
.SYNOPSIS
使用 Moon 免费报价接口检查视频请求参数和当前账号价格。
.DESCRIPTION
默认仅显示请求地址和正文大小，不读取密钥、不发送网络请求。-Quote 才会发送一次
POST /v1/video/quote；此接口不创建视频、不扣费，报价不锁定后续生成价格。
请求文件中的提示词和模型 ID 原样发送。脚本不重试、不跟随重定向、不回显失败响应，
成功报价会移除凭据字段并遮盖本次密钥。远程地址必须使用 HTTPS，HTTP 仅允许本机夹具。
.PARAMETER BaseUrl
Moon 根地址，默认 https://moon.sixai.cc；兼容末尾一次 /v1，可含反向代理前缀。
禁止 URL 内嵌凭据、查询参数和片段。
.PARAMETER RequestPath
本地 JSON 文件，正文必须为包含精确 model 字符串的对象，最大 1 MiB。
其他参数由 Moon 报价接口验证；不会打印提示词或参考素材。
.PARAMETER Quote
执行一次免费报价。密钥读取 MOON_API_KEY 环境变量；未设置时安全提示输入。
.PARAMETER RequestTimeoutSeconds
单次 HTTP 请求超时，单位秒，默认 60，范围 1 至 600。
.EXAMPLE
pwsh -File ./verification/moon-quote.ps1 -RequestPath ./request.json
仅检查本地输入并预览请求；无网络请求，也不会读取密钥。
.EXAMPLE
pwsh -File ./verification/moon-quote.ps1 -BaseUrl https://moon.sixai.cc/v1 -RequestPath ./request.json -Quote
安全读取密钥后调用一次免费报价；不创建视频任务。
.OUTPUTS
请求计划和脱敏后的供应商 JSON 报价。成功退出码为 0；输入、网络或供应商错误为 1。
#>
[CmdletBinding()]
param(
    [string]$BaseUrl = 'https://moon.sixai.cc',
    [Parameter(Mandatory)][string]$RequestPath,
    [switch]$Quote,
    [ValidateRange(1, 600)][int]$RequestTimeoutSeconds = 60
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Protect-MoonQuote {
    <#
    .SYNOPSIS
    递归移除响应中的凭据和请求正文，并遮盖字符串内的本次密钥。
    .PARAMETER Value
    已解析的 JSON 值；保留报价数量、价格及数组顺序。
    .PARAMETER Secret
    本次 Bearer 密钥，仅在内存中用于遮盖，不写入输出。
    .OUTPUTS
    脱敏后的 JSON 值。仅处理已受 64 层深度限制的响应，不修改输入对象。
    #>
    param([AllowNull()][object]$Value, [string]$Secret)
    if ($Value -is [System.Collections.IDictionary]) {
        $safe = [ordered]@{}
        foreach ($name in $Value.Keys) {
            $field = ([string]$name -replace '[-_]', '').ToLowerInvariant()
            if ($field -match '(?:^key$|^auth$|apikey|authorization|bearer|password|secret|credential|token$|cookie|sessionid|privatekey)' -or
                $field -in 'prompt', 'negativeprompt', 'request', 'requestbody', 'input', 'body', 'payload' -or
                ([string]$name).Contains($Secret)) { continue }
            $safe[$name] = Protect-MoonQuote -Value $Value[$name] -Secret $Secret
        }
        return $safe
    }
    if ($Value -is [System.Collections.IList]) {
        $safe = [System.Collections.Generic.List[object]]::new()
        foreach ($item in $Value) { $safe.Add((Protect-MoonQuote -Value $item -Secret $Secret)) }
        return ,$safe.ToArray()
    }
    if ($Value -is [string]) { return $Value.Replace($Secret, '[REDACTED]') }
    return $Value
}

$client = $null
$handler = $null
$request = $null
$response = $null
$token = $null
$secureToken = $null
$failureMessage = '本地报价检查失败，请检查输入文件和运行环境。'
try {
    $gateway = $null
    $failureMessage = 'BaseUrl 必须是无凭据、查询参数和片段的 HTTPS 地址；HTTP 仅允许本机地址。'
    if (-not [uri]::TryCreate($BaseUrl, [UriKind]::Absolute, [ref]$gateway) -or
        $gateway.Scheme -notin 'http', 'https' -or $BaseUrl -match '[\s\\?#]' -or $gateway.UserInfo -or
        ($gateway.Scheme -eq 'http' -and -not $gateway.IsLoopback)) { throw $failureMessage }
    $gatewayRoot = $gateway.AbsoluteUri.TrimEnd('/')
    if ($gatewayRoot.EndsWith('/v1', [StringComparison]::OrdinalIgnoreCase)) { $gatewayRoot = $gatewayRoot.Substring(0, $gatewayRoot.Length - 3) }
    $failureMessage = 'BaseUrl 只能包含末尾一次 /v1，不能填写视频接口路径或重复版本前缀。'
    if (([uri]$gatewayRoot).AbsolutePath -match '(?i)(?:^|/)v1(?:/|$)') { throw $failureMessage }
    $quoteUri = [uri]"$gatewayRoot/v1/video/quote"

    $failureMessage = 'RequestPath 必须是可读取的本地 JSON 文件。'
    $requestFile = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($RequestPath)
    $stream = $null
    $reader = $null
    try {
        $stream = [IO.File]::Open($requestFile, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
        $failureMessage = '请求 JSON 大小必须为 1 字节至 1 MiB。'
        if ($stream.Length -eq 0 -or $stream.Length -gt 1MB) { throw $failureMessage }
        $failureMessage = '请求文件不是有效的 UTF-8 JSON。'
        $reader = [IO.StreamReader]::new($stream, [Text.UTF8Encoding]::new($false, $true), $true)
        $body = $reader.ReadToEnd()
    }
    finally {
        if ($reader) { $reader.Dispose() }
        elseif ($stream) { $stream.Dispose() }
    }
    $failureMessage = '请求正文必须是有效的 JSON 对象，嵌套不得超过 64 层。'
    $payload = ConvertFrom-Json -InputObject $body -AsHashtable -Depth 64 -NoEnumerate
    if ($payload -isnot [System.Collections.IDictionary]) { throw $failureMessage }
    $failureMessage = '请求必须包含精确 model 字符串（1 至 200 个字母、数字或 ._:/-，不可包含空格）。'
    if ($payload['model'] -isnot [string] -or $payload['model'] -cnotmatch '^[A-Za-z0-9][A-Za-z0-9._:/-]{0,199}$') { throw $failureMessage }
    $bodyBytes = [Text.Encoding]::UTF8.GetByteCount($body)
    $failureMessage = '转换为 UTF-8 后的请求 JSON 不得超过 1 MiB。'
    if ($bodyBytes -gt 1MB) { throw $failureMessage }
    Write-Host "POST $quoteUri"
    Write-Host "Content-Type: application/json；已验证 model；正文 $bodyBytes 字节（不显示提示词、素材或凭据）"
    if (-not $Quote) {
        Write-Host '当前仅预览，未读取密钥、未发送请求。加 -Quote 调用一次免费报价。'
        return
    }

    $failureMessage = '无法读取 Moon 密钥，请设置 MOON_API_KEY 或在安全提示中输入。'
    $token = [Environment]::GetEnvironmentVariable('MOON_API_KEY')
    if ([string]::IsNullOrWhiteSpace($token)) {
        $secureToken = Read-Host '请输入 Moon API Key（不含 Bearer）' -AsSecureString
        $token = [Net.NetworkCredential]::new('', $secureToken).Password
    }
    $token = $token.Trim()
    $failureMessage = 'Moon 密钥格式无效，请输入原始 API Key，不含 Bearer、空格或换行。'
    if ($token.Length -eq 0 -or $token.Length -gt 2048 -or $token -cnotmatch '^[A-Za-z0-9._~+/-]+={0,2}$') { throw $failureMessage }
    $failureMessage = '无法初始化报价请求，请检查运行环境。'
    $handler = [Net.Http.HttpClientHandler]::new()
    $handler.AllowAutoRedirect = $false
    $handler.UseCookies = $false
    $handler.SslProtocols = [Security.Authentication.SslProtocols]::Tls12 -bor [Security.Authentication.SslProtocols]::Tls13
    if ($gateway.IsLoopback) { $handler.UseProxy = $false }
    $client = [Net.Http.HttpClient]::new($handler)
    $client.Timeout = [TimeSpan]::FromSeconds($RequestTimeoutSeconds)
    $client.MaxResponseContentBufferSize = 1MB
    $request = [Net.Http.HttpRequestMessage]::new([Net.Http.HttpMethod]::Post, $quoteUri)
    $request.Headers.Authorization = [Net.Http.Headers.AuthenticationHeaderValue]::new('Bearer', $token)
    $request.Content = [Net.Http.StringContent]::new($body, [Text.Encoding]::UTF8, 'application/json')
    $failureMessage = '报价请求未完成，请检查网络、证书或供应商服务；未自动重试。'
    $response = $client.SendAsync($request).GetAwaiter().GetResult()
    $status = [int]$response.StatusCode
    if (-not $response.IsSuccessStatusCode) {
        $failureMessage = "HTTP ${status}：报价失败，请检查请求参数、账号权限及 Moon 服务；未自动重试。"
        if ($status -ge 300 -and $status -lt 400) { $failureMessage = "HTTP ${status}：已拒绝重定向，请检查最终 Moon 地址；未自动重试。" }
        if ($status -in 401, 403) { $failureMessage = "HTTP ${status}：请检查 Moon API Key 和账号权限；未自动重试。" }
        throw $failureMessage
    }
    $failureMessage = '供应商报价响应不是有效的 JSON 对象；未显示响应正文，未自动重试。'
    $receipt = ConvertFrom-Json -InputObject $response.Content.ReadAsStringAsync().GetAwaiter().GetResult() -AsHashtable -Depth 64 -NoEnumerate
    if ($receipt -isnot [System.Collections.IDictionary]) { throw $failureMessage }
    $failureMessage = 'Moon 返回报价错误，请检查请求参数和账号状态；未显示响应正文，未自动重试。'
    if ($null -ne $receipt['error'] -or $receipt['success'] -ceq $false) { throw $failureMessage }
    $failureMessage = '无法安全显示供应商报价，未显示响应正文。'
    $safeReceipt = Protect-MoonQuote -Value $receipt -Secret $token
    $quoteJson = ConvertTo-Json -InputObject $safeReceipt -Depth 64
    Write-Host 'Moon 报价成功（未创建视频，未扣费；报价不锁定后续生成价格）：'
    Write-Output $quoteJson
}
catch {
    [Console]::Error.WriteLine("Moon 报价检查未完成：$failureMessage")
    exit 1
}
finally {
    if ($response) { $response.Dispose() }
    if ($request) { $request.Dispose() }
    if ($client) { $client.Dispose() }
    elseif ($handler) { $handler.Dispose() }
    if ($secureToken) { $secureToken.Dispose() }
    $token = $null
}
