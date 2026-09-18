#Requires -Version 7.0

<#
.SYNOPSIS
通过 new-api 网关验证一次异步视频生成，并保留可恢复的任务检查点。
.DESCRIPTION
新任务默认只显示请求计划；-Submit 才会发送一次 POST /v1/videos，可能产生费用。
检查点在发送前以独占新建方式保存。已有检查点或 -TaskId 只执行 GET；提交结果未知时
必须先在网关任务日志中查找原任务，不能删除检查点后盲目重新生成。
本脚本拒绝 HTTP 重定向，不会将网关令牌发送到视频 CDN。下载要求 MP4 媒体类型或
application/octet-stream，并验证非空文件和 MP4 文件头；不会覆盖已有输出文件。
.PARAMETER BaseUrl
new-api 网关根地址，可包含反向代理前缀，不要填写供应商地址或末尾的 /v1。
.PARAMETER ChannelId
管理员所属 API 令牌固定使用的渠道编号。脚本在令牌后附加 -<渠道编号>。
.PARAMETER Model
发送给网关的精确模型名，默认 wan3.0-video；恢复时使用检查点中的原任务。
.PARAMETER Seconds
视频时长，单位秒，范围 2 至 30；默认 2，具体模型仍以供应商限制为准。
.PARAMETER Resolution
输出分辨率，可选 480p、720p、1080p，默认 480p。
.PARAMETER Prompt
视频提示词，默认英文海浪场景。自定义提示词建议使用英文，不要放入密钥。
.PARAMETER Submit
允许新建一次真实任务；已有检查点时仍不会重新提交。
.PARAMETER TaskId
已有的网关公开任务编号；仅恢复查询，也可补充结果未知的检查点。
.PARAMETER CheckpointPath
本地 JSON 检查点，不包含令牌、提示词、供应商响应或下载链接。
.PARAMETER OutputPath
下载目标 MP4 文件，不覆盖已有文件；默认位于检查点同目录。
.PARAMETER PollIntervalSeconds
查询间隔，单位秒，默认 5，范围 1 至 60。
.PARAMETER TimeoutSeconds
本次轮询时间预算，单位秒，默认 600；到期后可以用同一检查点恢复。
.PARAMETER RequestTimeoutSeconds
单次 HTTP 请求或下载的超时，单位秒，默认 60，范围 1 至 600。
.EXAMPLE
pwsh -File ./verification/video-smoke.ps1 -BaseUrl https://gateway.example -ChannelId 12
预览两秒 480p 请求，不读取令牌、不发送 HTTP 请求、不创建检查点。
.EXAMPLE
pwsh -File ./verification/video-smoke.ps1 -BaseUrl https://gateway.example -ChannelId 12 -Submit
从 NEW_API_TEST_TOKEN 环境变量读取原始 API 令牌；未设置时安全提示输入，然后创建一次任务。
.EXAMPLE
pwsh -File ./verification/video-smoke.ps1 -BaseUrl https://gateway.example -ChannelId 12 -TaskId task_example
仅查询已有任务并下载；也可省略 -TaskId，直接使用默认检查点恢复。
.OUTPUTS
中文执行状态。成功时保存 MP4 和检查点，退出码为 0；失败或超时退出码为 1。
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$BaseUrl,
    [Parameter(Mandatory)][ValidateRange(1, 2147483647)][int]$ChannelId,
    [ValidatePattern('^[A-Za-z0-9][A-Za-z0-9._:/-]{0,199}$')][string]$Model = 'wan3.0-video',
    [ValidateRange(2, 30)][int]$Seconds = 2,
    [ValidateSet('480p', '720p', '1080p')][string]$Resolution = '480p',
    [ValidateNotNullOrEmpty()][string]$Prompt = 'A calm ocean wave under soft daylight, static camera, no text',
    [switch]$Submit,
    [string]$TaskId,
    [string]$CheckpointPath = (Join-Path $PSScriptRoot '../.local-tests/moon-video/checkpoint.json'),
    [string]$OutputPath,
    [ValidateRange(1, 60)][int]$PollIntervalSeconds = 5,
    [ValidateRange(1, 86400)][int]$TimeoutSeconds = 600,
    [ValidateRange(1, 600)][int]$RequestTimeoutSeconds = 60
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Save-VideoCheckpoint {
    <#
    .SYNOPSIS
    将不含凭据的状态持久化；首次写入禁止覆盖，后续更新通过同目录临时文件原子替换。
    .PARAMETER Path
    检查点的绝对文件路径。
    .PARAMETER State
    仅包含网关、渠道、任务和本地制品信息的字典。
    .PARAMETER Create
    首次创建；文件已存在时抛出错误，调用方不得继续提交。
    .OUTPUTS
    无。写入或落盘失败时抛出异常，可能留下可供人工核查的原检查点。
    #>
    param([string]$Path, [System.Collections.IDictionary]$State, [switch]$Create)
    $writePath = if ($Create) { $Path } else { "$Path.$([Guid]::NewGuid().ToString('N')).tmp" }
    $stream = $null
    try {
        $bytes = [Text.Encoding]::UTF8.GetBytes(($State | ConvertTo-Json -Depth 6))
        $stream = [IO.File]::Open($writePath, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::Read)
        $stream.Write($bytes, 0, $bytes.Length)
        $stream.Flush($true)
        $stream.Dispose()
        $stream = $null
        if (-not $Create) { [IO.File]::Move($writePath, $Path, $true) }
    }
    finally {
        if ($stream) { $stream.Dispose() }
    }
}

function Invoke-VideoRequest {
    <#
    .SYNOPSIS
    发送一次网关请求，不重试，不跟随重定向，不回显上游错误体或请求头。
    .PARAMETER Client
    已禁用重定向并设置网关认证的 HTTP 客户端，由调用方释放。
    .PARAMETER Uri
    由已验证网关地址和固定协议路径构造的请求地址。
    .PARAMETER Method
    GET 或 POST。
    .PARAMETER Body
    POST 的 JSON 正文，GET 时留空。
    .PARAMETER Download
    仅缓冲响应头，供调用方以独立超时流式下载。
    .OUTPUTS
    成功的 HttpResponseMessage，由调用方释放；网络、重定向和 HTTP 错误抛出中文异常。
    #>
    param([Net.Http.HttpClient]$Client, [uri]$Uri, [string]$Method = 'GET', [string]$Body, [switch]$Download)
    $request = [Net.Http.HttpRequestMessage]::new([Net.Http.HttpMethod]::new($Method), $Uri)
    try {
        if ($Method -eq 'POST') { $request.Content = [Net.Http.StringContent]::new($Body, [Text.Encoding]::UTF8, 'application/json') }
        $completion = if ($Download) { [Net.Http.HttpCompletionOption]::ResponseHeadersRead } else { [Net.Http.HttpCompletionOption]::ResponseContentRead }
        try { $response = $Client.SendAsync($request, $completion).GetAwaiter().GetResult() }
        catch { throw 'HTTP 请求未完成。请检查网络及网关日志；保留检查点，查到原任务编号后用 -TaskId 恢复，勿重新生成。' }
        if ($response.IsSuccessStatusCode) { return $response }
        $status = [int]$response.StatusCode
        $response.Dispose()
        if ($status -ge 300 -and $status -lt 400) { throw '网关返回重定向，已拒绝跳转以保护令牌。请检查最终网关地址及反向代理配置；保留原检查点。' }
        if ($status -in 401, 403) { throw "HTTP $status：请检查管理员所属 API 令牌、渠道权限和令牌限制；修复后恢复原任务。" }
        throw "HTTP $status：请在网关任务/请求日志中检查渠道、模型、价格、余额及供应商错误；保留检查点，不自动重新生成。"
    }
    finally { $request.Dispose() }
}

$client = $null
$handler = $null
$token = $null
$secureToken = $null
$partialPath = $null
try {
    $gateway = $null
    if (-not [uri]::TryCreate($BaseUrl, [UriKind]::Absolute, [ref]$gateway) -or
        $gateway.Scheme -notin 'http', 'https' -or $BaseUrl -match '[\s\\?#]' -or $gateway.UserInfo) {
        throw 'BaseUrl 必须是无凭据、查询参数和片段的 HTTP(S) 网关地址。'
    }
    $gatewayRoot = $gateway.AbsoluteUri.TrimEnd('/')
    if ($gateway.AbsolutePath.TrimEnd('/') -match '/v1(?:/.*)?$') { throw 'BaseUrl 应为 new-api 网关根地址，不要附加 /v1 或视频接口路径。' }
    if ($TaskId -and $TaskId -notmatch '^[A-Za-z0-9][A-Za-z0-9_.-]{0,199}$') { throw 'TaskId 必须是网关返回的公开任务编号，不能包含路径或查询参数。' }
    if ([string]::IsNullOrWhiteSpace($Prompt)) { throw 'Prompt 不能为空白。' }

    $checkpointFile = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($CheckpointPath)
    if ([IO.Path]::GetExtension($checkpointFile) -ne '.json' -or [IO.Directory]::Exists($checkpointFile)) { throw 'CheckpointPath 必须指向本地 .json 文件。' }
    $checkpoint = $null
    if ([IO.File]::Exists($checkpointFile)) {
        try { $checkpoint = Get-Content -LiteralPath $checkpointFile -Raw | ConvertFrom-Json -AsHashtable }
        catch { throw '检查点不可读取或不是有效 JSON。请保留文件并在网关日志中查找原任务，勿重新提交。' }
        if ($checkpoint -isnot [System.Collections.IDictionary] -or $checkpoint['version'] -ne 1 -or
            $checkpoint['baseUrl'] -cne $gatewayRoot -or $checkpoint['channelId'] -ne $ChannelId) {
            throw '检查点版本、网关或渠道不匹配。请使用原网关和渠道恢复，或为独立的新测试指定新的检查点路径。'
        }
        $savedId = $checkpoint['taskId']
        if ($savedId -and ($savedId -isnot [string] -or $savedId -notmatch '^[A-Za-z0-9][A-Za-z0-9_.-]{0,199}$')) { throw '检查点任务编号无效，请核对网关任务日志。' }
        if ($TaskId -and $savedId -and $TaskId -cne $savedId) { throw 'TaskId 与检查点中的任务不同；请为另一个任务指定独立检查点。' }
        if (-not $TaskId) { $TaskId = $savedId }
        if (-not $TaskId) { throw '已有提交检查点，但尚未记录任务编号，提交结果未知。请先在网关任务日志核对，再以 -TaskId 原任务编号恢复；不会再次 POST。' }
        # 只保留约定字段，避免将人工添加的凭据或供应商响应再次写入检查点。
        $checkpoint = [ordered]@{
            version = 1; baseUrl = $gatewayRoot; channelId = $ChannelId
            model = $checkpoint['model']; seconds = $checkpoint['seconds']; resolution = $checkpoint['resolution']
            createdAt = $checkpoint['createdAt']; taskId = $TaskId; phase = $checkpoint['phase']
            outputPath = $checkpoint['outputPath']; sha256 = $checkpoint['sha256']; bytes = $checkpoint['bytes']
        }
    }
    if (-not $OutputPath) {
        $OutputPath = if ($checkpoint -and $checkpoint['outputPath']) { $checkpoint['outputPath'] } else { Join-Path ([IO.Path]::GetDirectoryName($checkpointFile)) 'video.mp4' }
    }
    $outputFile = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($OutputPath)
    if ([IO.Path]::GetExtension($outputFile) -ne '.mp4' -or [IO.Directory]::Exists($outputFile)) { throw 'OutputPath 必须指向本地 .mp4 文件。' }
    if ([IO.File]::Exists($outputFile)) {
        if ($checkpoint -and $checkpoint['phase'] -eq 'completed' -and $checkpoint['outputPath'] -eq $outputFile -and
            (Get-FileHash -LiteralPath $outputFile -Algorithm SHA256).Hash -eq $checkpoint['sha256']) {
            Write-Host "视频已下载，SHA256 校验通过：$outputFile"
            return
        }
        throw '输出文件已存在，不会覆盖。请使用其他 -OutputPath，保留同一检查点恢复原任务。'
    }

    $body = [ordered]@{ model = $Model; prompt = $Prompt; seconds = $Seconds; resolution = $Resolution } | ConvertTo-Json -Depth 4
    $createUri = [uri]"$gatewayRoot/v1/videos"
    if (-not $TaskId -and -not $Submit) {
        Write-Host "预览：POST $createUri"
        Write-Host "Authorization: Bearer <已隐藏的管理员 API 令牌>-$ChannelId"
        Write-Host 'Content-Type: application/json'
        Write-Host $body
        Write-Host "检查点：$checkpointFile"
        Write-Host "视频文件：$outputFile"
        Write-Host '未发送 HTTP 请求。确认渠道、模型价格和额度后，加 -Submit 执行一次真实生成。'
        return
    }

    $token = [Environment]::GetEnvironmentVariable('NEW_API_TEST_TOKEN')
    if ([string]::IsNullOrWhiteSpace($token)) {
        $secureToken = Read-Host '请输入管理员所属 API 令牌（不含 Bearer 和渠道后缀）' -AsSecureString
        $token = [Net.NetworkCredential]::new('', $secureToken).Password
    }
    $token = $token.Trim()
    if ($token.Length -gt 512 -or $token -notmatch '^(?:sk-)?[A-Za-z0-9]+$') { throw '令牌格式无效。请输入原始 API 令牌，不含 Bearer、空格或渠道后缀。' }
    $handler = [Net.Http.HttpClientHandler]::new()
    $handler.AllowAutoRedirect = $false
    $handler.UseCookies = $false
    $client = [Net.Http.HttpClient]::new($handler)
    $client.Timeout = [TimeSpan]::FromSeconds($RequestTimeoutSeconds)
    $client.MaxResponseContentBufferSize = 1MB
    $client.DefaultRequestHeaders.Authorization = [Net.Http.Headers.AuthenticationHeaderValue]::new('Bearer', "$token-$ChannelId")
    [void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($checkpointFile))
    [void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($outputFile))
    if (-not $checkpoint) {
        $checkpoint = [ordered]@{
            version = 1; baseUrl = $gatewayRoot; channelId = $ChannelId
            model = $Model; seconds = $Seconds; resolution = $Resolution
            createdAt = [DateTimeOffset]::UtcNow.ToString('o'); taskId = $TaskId
            phase = $(if ($TaskId) { 'task_known' } else { 'submission_pending' })
        }
        Save-VideoCheckpoint -Path $checkpointFile -State $checkpoint -Create
    }

    if (-not $TaskId) {
        Write-Host '检查点已保存，正在提交一次视频任务。请求失败或中断后不会自动重发。'
        $response = Invoke-VideoRequest -Client $client -Uri $createUri -Method POST -Body $body
        try {
            try { $receipt = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult() | ConvertFrom-Json -AsHashtable }
            catch { throw '提交响应不是有效 JSON，受理结果未知。请根据检查点时间在网关任务日志查找原任务，再用 -TaskId 恢复。' }
        }
        finally { $response.Dispose() }
        if ($receipt -isnot [System.Collections.IDictionary] -or $receipt['id'] -isnot [string] -or
            $receipt['id'] -notmatch '^[A-Za-z0-9][A-Za-z0-9_.-]{0,199}$') {
            throw '提交响应缺少有效 id，受理结果未知。请先在网关任务日志找到原任务，再用 -TaskId 恢复；不会重新提交。'
        }
        $TaskId = $receipt['id']
        Write-Host "已受理任务：$TaskId（尚未确认生成成功）"
    }
    $checkpoint['taskId'] = $TaskId
    $checkpoint['phase'] = 'task_known'
    Save-VideoCheckpoint -Path $checkpointFile -State $checkpoint
    $taskUri = [uri]"$gatewayRoot/v1/videos/$TaskId"
    $timer = [Diagnostics.Stopwatch]::StartNew()
    while ($true) {
        $response = Invoke-VideoRequest -Client $client -Uri $taskUri
        try {
            try { $task = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult() | ConvertFrom-Json -AsHashtable }
            catch { throw '任务查询未返回有效 JSON。请检查网关日志，保留检查点后重新运行以恢复查询。' }
        }
        finally { $response.Dispose() }
        if ($task -isnot [System.Collections.IDictionary] -or $task['id'] -cne $TaskId) { throw '任务查询响应的 id 不匹配。请检查网关地址和任务编号，保留检查点。' }
        $status = $task['status']
        if ($status -notin 'queued', 'in_progress', 'completed', 'failed') { throw '任务查询返回未知状态。请核对网关版本和任务日志；不会将未知状态视为成功。' }
        Write-Host "任务 $TaskId：$status"
        if ($status -eq 'failed') {
            $checkpoint['phase'] = 'failed'
            Save-VideoCheckpoint -Path $checkpointFile -State $checkpoint
            throw '生成失败。请在网关任务日志查看该任务的失败原因和结算记录；修复渠道或请求后，另设检查点才能开始新的付费测试。'
        }
        if ($status -eq 'completed') { break }
        $remaining = $TimeoutSeconds - $timer.Elapsed.TotalSeconds
        if ($remaining -le 0) { throw '本次轮询超时，任务可能仍在生成。用相同网关、渠道和检查点重新运行即可继续 GET 查询，不要重新生成。' }
        Start-Sleep -Milliseconds ([int][Math]::Min($PollIntervalSeconds * 1000, $remaining * 1000))
    }

    $response = Invoke-VideoRequest -Client $client -Uri ([uri]"$taskUri/content") -Download
    $downloadTimeout = [Threading.CancellationTokenSource]::new([TimeSpan]::FromSeconds($RequestTimeoutSeconds))
    $file = $null
    try {
        $contentType = $response.Content.Headers.ContentType
        $mediaType = if ($contentType) { $contentType.MediaType } else { '' }
        if ([int]$response.StatusCode -ne 200 -or $mediaType -notin 'video/mp4', 'application/mp4', 'application/octet-stream') {
            throw '下载未返回完整 MP4 媒体响应，可能是网关或供应商错误。未保存为视频；保留检查点后恢复下载。'
        }
        $partialPath = "$outputFile.$([Guid]::NewGuid().ToString('N')).part"
        $file = [IO.File]::Open($partialPath, [IO.FileMode]::CreateNew, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
        try { $response.Content.CopyToAsync($file, $downloadTimeout.Token).GetAwaiter().GetResult() }
        catch { throw '视频下载未完成。保留检查点并重新运行即可恢复查询和下载，不会重新创建任务。' }
        $file.Flush($true)
        $length = $file.Length
        $header = [byte[]]::new(12)
        $file.Position = 0
        $read = $file.Read($header, 0, $header.Length)
        if ($read -lt 12 -or [Text.Encoding]::ASCII.GetString($header, 4, 4) -cne 'ftyp') { throw '下载内容为空或不包含 MP4 文件头，未保存为成功视频。请检查网关制品和供应商结果。' }
        $file.Dispose()
        $file = $null
        [IO.File]::Move($partialPath, $outputFile, $false)
        $partialPath = $null
    }
    finally {
        if ($file) { $file.Dispose() }
        $downloadTimeout.Dispose()
        $response.Dispose()
    }
    $checkpoint['phase'] = 'completed'
    $checkpoint['outputPath'] = $outputFile
    $checkpoint['bytes'] = $length
    $checkpoint['sha256'] = (Get-FileHash -LiteralPath $outputFile -Algorithm SHA256).Hash
    Save-VideoCheckpoint -Path $checkpointFile -State $checkpoint
    Write-Host "生成完成并已下载：$outputFile（$length 字节）"
    Write-Host "检查点：$checkpointFile"
}
catch {
    [Console]::Error.WriteLine("视频测试未完成：$($_.Exception.Message)")
    exit 1
}
finally {
    if ($partialPath -and [IO.File]::Exists($partialPath)) { [IO.File]::Delete($partialPath) }
    if ($client) { $client.Dispose() }
    elseif ($handler) { $handler.Dispose() }
    if ($secureToken) { $secureToken.Dispose() }
    $token = $null
}
