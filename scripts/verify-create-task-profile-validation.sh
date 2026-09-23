#!/usr/bin/env bash
# Verifies REQ-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001 against a running
# Kandev instance: an unresolvable agent_profile_id is refused and creates
# nothing. Needs no agent and no browser.
set -u
BASE=${BASE:?set BASE to the running Kandev instance URL, e.g. http://localhost:28096}
H='Content-Type: application/json'; A='Accept: application/json, text/event-stream'
SID=$(curl -s -X POST "$BASE/mcp" -H "$H" -H "$A" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-03-26","capabilities":{},"clientInfo":{"name":"probe","version":"1"}}}' \
  -D - -o /dev/null | grep -i '^Mcp-Session-Id' | tr -d '\r' | cut -d' ' -f2)
curl -s -X POST "$BASE/mcp" -H "$H" -H "$A" -H "Mcp-Session-Id: $SID" \
  -d '{"jsonrpc":"2.0","method":"notifications/initialized"}' -o /dev/null
call(){ curl -s -X POST "$BASE/mcp" -H "$H" -H "$A" -H "Mcp-Session-Id: $SID" -d "$1"; }
txt(){ python3 -c 'import sys,json;d=json.load(sys.stdin);r=d.get("result",{});print(r.get("isError"),"|",((r.get("content") or [{}])[0].get("text","")).strip()[:220])'; }

WSID=$(call '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_workspaces_kandev","arguments":{}}}' \
  | python3 -c 'import sys,json;d=json.load(sys.stdin);print(json.loads(d["result"]["content"][0]["text"])["workspaces"][0]["id"])')
WF=$(call "{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"tools/call\",\"params\":{\"name\":\"list_workflows_kandev\",\"arguments\":{\"workspace_id\":\"$WSID\"}}}" \
  | python3 -c 'import sys,json;d=json.load(sys.stdin);print(json.loads(d["result"]["content"][0]["text"])["workflows"][0]["id"])')
cnt(){ call "{\"jsonrpc\":\"2.0\",\"id\":4,\"method\":\"tools/call\",\"params\":{\"name\":\"list_tasks_kandev\",\"arguments\":{\"workflow_id\":\"$WF\"}}}" \
  | python3 -c 'import sys,json;d=json.load(sys.stdin);print(json.loads(d["result"]["content"][0]["text"]).get("total"))'; }

echo "tasks before: $(cnt)"
for V in current_task workspace_default 11111111-2222-3333-4444-555555555555; do
  echo; echo "agent_profile_id=\"$V\""
  call "{\"jsonrpc\":\"2.0\",\"id\":5,\"method\":\"tools/call\",\"params\":{\"name\":\"create_task_kandev\",\"arguments\":{\"title\":\"probe\",\"agent_profile_id\":\"$V\",\"workflow_id\":\"$WF\",\"start_agent\":false}}}" | txt
done
echo; echo "tasks after:  $(cnt)   <- must equal 'before'"
