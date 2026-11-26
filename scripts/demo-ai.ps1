#requires -Version 5.1
[CmdletBinding()]
param(
    [uri]$BaseUrl = 'http://127.0.0.1:18080/api/v1',
    [PSCredential]$AdminCredential,
    [switch]$LocalMock,
    [switch]$SimulateReview
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if (-not $LocalMock) { throw 'This demo makes three analysis calls. Point the demo API at mock-ai -demo-outage, then specify -LocalMock.' }
. (Join-Path $PSScriptRoot 'demo-common.ps1')

function New-ClaimedTicket($Title) {
    $row = Invoke-Api POST '/tickets' 201 $userToken @{title=$Title; description='Synthetic demo: the invoice preview is missing.'}
    Confirm-Step ($row.id -gt 0 -and $row.requester_id -eq $user.id -and $row.status -eq 'open') 'ticket created'
    $assigned = Invoke-Api POST "/tickets/$($row.id)/claim" 200 $agentToken $null
    Confirm-Step ($assigned.status -eq 'in_progress' -and $assigned.assignee_id -eq $agent.id) 'agent claimed ticket'
    return "/tickets/$($row.id)"
}

function Confirm-CurrentSuggestion($Run) {
    Confirm-Step ($Run.status -ceq 'succeeded' -and $Run.is_stale -is [bool] -and !$Run.is_stale -and $Run.id -gt 0) 'fresh successful analysis'
    Confirm-Step ($Run.result.category -cin @('general','account','billing','technical') -and
        ![string]::IsNullOrWhiteSpace($Run.result.summary) -and ![string]::IsNullOrWhiteSpace($Run.result.reply_suggestion)) 'validated suggestion'
}

$handler = [Net.Http.HttpClientHandler]::new()
$handler.AllowAutoRedirect = $false
$client = [Net.Http.HttpClient]::new($handler)
$client.Timeout = [TimeSpan]::FromSeconds(30)
try {
    Write-Host 'Local mock demo creates accounts, tickets, messages and analysis history.'
    $suffix = [Guid]::NewGuid().ToString('N')
    $userEmail = "user-$suffix@example.test"
    $userPass = New-DemoPassword
    $user = Invoke-Api POST '/auth/register' 201 $null @{email=$userEmail; password=$userPass; display_name='AI Demo User'}
    Confirm-Step ($user.role -eq 'user' -and $user.id -gt 0) 'user registered'
    $userToken = Connect-Account $userEmail $userPass 'user'
    $adminToken = Connect-Account $AdminCredential.UserName $AdminCredential.GetNetworkCredential().Password 'admin'
    $agentPass = New-DemoPassword
    $agent = Invoke-Api POST '/admin/staff' 201 $adminToken @{email="agent-$suffix@example.test"; password=$agentPass; role='agent'; display_name='AI Demo Agent'}
    Confirm-Step ($agent.role -eq 'agent' -and $agent.id -gt 0) 'agent created'
    $agentToken = Connect-Account $agent.email $agentPass 'agent'

    $path = New-ClaimedTicket 'A3 invoice preview'
    $first = Invoke-Api POST "$path/ai-runs" 201 $agentToken $null
    Confirm-CurrentSuggestion $first
    $update = Invoke-Api POST "$path/messages" 201 $userToken @{content='The preview is also missing in a second browser.'}
    Confirm-Step ($update.author_id -eq $user.id) 'user added context'
    $old = Invoke-Api GET "$path/ai-runs/$($first.id)" 200 $agentToken $null
    Confirm-Step ($old.is_stale -eq $true -and $old.input_snapshot_id -ceq $first.input_snapshot_id) 'old suggestion is stale'
    $next = Invoke-Api POST "$path/ai-runs" 201 $agentToken $null
    Confirm-CurrentSuggestion $next
    $review = Invoke-Api GET "$path/ai-runs/$($next.id)" 200 $agentToken $null
    Confirm-CurrentSuggestion $review
    Confirm-Step ($review.id -ne $old.id -and $review.input_snapshot_id -cne $old.input_snapshot_id) 'independent fresh analysis'
    $draft = $review.result.reply_suggestion
    if ($SimulateReview) {
        Write-Host 'Automated fixture: simulating an edit and send; no human approval is recorded.'
        $edited = "$draft Please confirm which invoice preview you opened."
    } else {
        Write-Host "Summary: $($review.result.summary)"
        Write-Host "Suggested reply: $draft"
        $edited = Read-Host 'Enter your edited reply'
        if ([string]::IsNullOrWhiteSpace($edited) -or $edited.Trim() -ceq $draft.Trim()) { throw 'An edited reply is required.' }
        Write-Host "Reply to send: $edited"
        if ((Read-Host 'Type SEND to confirm this reply') -cne 'SEND') { throw 'Staff reply canceled.' }
    }
    $current = Invoke-Api GET "$path/ai-runs/$($next.id)" 200 $agentToken $null
    Confirm-CurrentSuggestion $current
    $sent = Invoke-Api POST "$path/messages" 201 $agentToken @{content=$edited}
    Confirm-Step ($sent.author_id -eq $agent.id -and $sent.content -ceq $edited) 'normal agent reply'
    $history = Invoke-Api GET "$path/messages" 200 $userToken $null
    Confirm-Step ($history.total -eq 2 -and $history.items[1].id -eq $sent.id) 'only user and staff messages sent'

    $fault = New-ClaimedTicket 'A3 synthetic provider outage'
    $failed = Invoke-Api POST "$fault/ai-runs" 502 $agentToken $null
    Confirm-Step ($failed.code -eq 'ai_analysis_failed' -and $failed.request_id) 'controlled upstream failure'
    $manual = Invoke-Api POST "$fault/messages" 201 $agentToken @{content='I will check the preview with you manually.'}
    Confirm-Step ($manual.author_id -eq $agent.id) 'manual fallback reply'
    $resolved = Invoke-Api POST "$fault/resolve" 200 $agentToken $null
    $closed = Invoke-Api POST "$fault/close" 200 $userToken $null
    Confirm-Step ($resolved.status -eq 'resolved' -and $closed.status -eq 'closed') 'manual resolution and closure'
    [pscustomobject]@{FirstRun=$first.id; SecondRun=$next.id; AgentMessage=$sent.id; FailureTicket=$closed.id; SimulatedReview=[bool]$SimulateReview}
} finally {
    $client.Dispose()
}
