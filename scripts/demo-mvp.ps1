#requires -Version 5.1
[CmdletBinding()]
param(
    [uri]$BaseUrl = 'http://127.0.0.1:18080/api/v1',
    [PSCredential]$AdminCredential
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot 'demo-common.ps1')

function New-DemoTicket($Title) {
    $row = Invoke-Api POST '/tickets' 201 $userToken @{title=$Title; description='MVP demonstration'}
    Confirm-Step ($row.id -gt 0 -and $row.requester_id -eq $user.id -and $row.status -eq 'open' -and $null -eq $row.assignee_id) 'create ticket'
    return $row.id
}

function Add-Reply($Token, $Author, $Text) {
    $msg = Invoke-Api POST "$path/messages" 201 $Token @{content=$Text}
    Confirm-Step ($msg.id -gt 0 -and $msg.ticket_id -eq $ticketId -and $msg.author_id -eq $Author -and $msg.content -ceq $Text) 'reply'
    return $msg.id
}

function Set-TicketAction($Action, $Token, $Status) {
    $row = Invoke-Api POST "$path/$Action" 200 $Token $null
    Confirm-Step ($row.id -eq $ticketId -and $row.status -eq $Status -and $row.assignee_id -eq $agents[0].id) $Action
}



$handler = [Net.Http.HttpClientHandler]::new()
$handler.AllowAutoRedirect = $false
$client = [Net.Http.HttpClient]::new($handler)
$client.Timeout = [TimeSpan]::FromSeconds(15)
try {
    Write-Host "Creating demo accounts and tickets at $BaseUrl. Records are retained."
    $run = [Guid]::NewGuid().ToString('N')
    $userEmail = "user-$run@example.test"
    $userPass = New-DemoPassword
    $user = Invoke-Api POST '/auth/register' 201 $null @{email=$userEmail; password=$userPass; display_name='Demo User'}
    Confirm-Step ($user.id -gt 0 -and $user.email -eq $userEmail -and $user.role -eq 'user') 'registration'
    $adminToken = Connect-Account $AdminCredential.UserName $AdminCredential.GetNetworkCredential().Password 'admin'
    $agents = @()
    $agentTokens = @()
    $passwords = @((New-DemoPassword), (New-DemoPassword))
    foreach ($i in 0,1) {
        $email = "agent-$i-$run@example.test"
        $row = Invoke-Api POST '/admin/staff' 201 $adminToken @{email=$email; password=$passwords[$i]; display_name="Demo Agent $i"; role='agent'}
        Confirm-Step ($row.id -gt 0 -and $row.role -eq 'agent' -and $row.email -eq $email) 'create agent'
        $agents += $row
    }
    $userToken = Connect-Account $userEmail $userPass 'user'
    foreach ($i in 0,1) { $agentTokens += Connect-Account $agents[$i].email $passwords[$i] 'agent' }

    $ticketId = New-DemoTicket "Login issue $run"
    $path = "/tickets/$ticketId"
    $page = 1
    do {
        $queue = Invoke-Api GET "/tickets/queue?page=$page&page_size=100" 200 $agentTokens[0] $null
        $found = $queue.items.id -contains $ticketId
        $page++
    } while (-not $found -and ($page-1)*100 -lt $queue.total)
    Confirm-Step $found 'unassigned queue'
    Set-TicketAction claim $agentTokens[0] 'in_progress'
    $messageIds = @((Add-Reply $userToken $user.id 'Still unable to log in'), (Add-Reply $agentTokens[0] $agents[0].id 'Please try again'))
    $updated = Invoke-Api PATCH "$path/priority" 200 $agentTokens[0] @{priority='high'}
    Confirm-Step ($updated.priority -eq 'high' -and $updated.id -eq $ticketId) 'priority'
    Set-TicketAction resolve $agentTokens[0] 'resolved'
    Set-TicketAction reopen $userToken 'in_progress'
    $messageIds += Add-Reply $userToken $user.id 'Now working after retry'
    Set-TicketAction resolve $agentTokens[0] 'resolved'
    Set-TicketAction close $userToken 'closed'
    $final = Invoke-Api GET $path 200 $userToken $null
    Confirm-Step ($final.status -eq 'closed' -and $final.priority -eq 'high' -and $final.assignee_id -eq $agents[0].id) 'closed ticket'
    $messages = Invoke-Api GET "$path/messages" 200 $userToken $null
    Confirm-Step ($messages.total -eq 3 -and ($messages.items.id -join ',') -eq ($messageIds -join ',')) 'ordered message IDs'
    $events = Invoke-Api GET "$path/events" 200 $userToken $null
    $order = 'created,claimed,message_added,message_added,priority_changed,resolved,reopened,message_added,resolved,closed'
    Confirm-Step ($events.total -eq 10 -and ($events.items.event_type -join ',') -eq $order) 'lifecycle events'
    $eventIds = @($events.items | Where-Object event_type -eq 'message_added' | ForEach-Object { $_.metadata.message_id })
    Confirm-Step (($eventIds -join ',') -eq ($messageIds -join ',')) 'message event references'

    $assignedId = New-DemoTicket "Transfer $run"
    $second = "/tickets/$assignedId"
    foreach ($agent in $agents) {
        $row = Invoke-Api PUT "$second/assignee" 200 $adminToken @{assignee_id=$agent.id}
        Confirm-Step ($row.id -eq $assignedId -and $row.assignee_id -eq $agent.id -and $row.status -eq 'in_progress') 'assignment'
    }
    foreach ($suffix in '', '/messages', '/events') {
        $null = Invoke-Api GET "$second$suffix" 404 $agentTokens[0] $null
    }
    $current = Invoke-Api GET $second 200 $agentTokens[1] $null
    Confirm-Step ($current.assignee_id -eq $agents[1].id) 'new assignee access'
    $history = Invoke-Api GET "$second/events" 200 $agentTokens[1] $null
    Confirm-Step (($history.items.event_type -join ',') -eq 'created,assigned,reassigned') 'assignment history'
    [pscustomobject]@{ClosedTicketId=$ticketId; TransferredTicketId=$assignedId; Messages=$messages.total; Events=$events.total}
} finally {
    $client.Dispose()
}
