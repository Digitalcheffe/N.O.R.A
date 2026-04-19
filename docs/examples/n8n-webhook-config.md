# n8n Webhook Configuration for NORA

## Setup Instructions

### Step 1 — Create a global Error Workflow

This catches failures from all your workflows automatically without touching each one.

1. In n8n create a new workflow and name it something like **NORA Error Handler**
2. Add a **Start** trigger node (it will receive error data automatically)
3. Add an **HTTP Request** node with the error payload below
4. Save the workflow and copy its ID from the URL bar
5. Go to **Settings → Error Workflow** and select this workflow
6. Now any workflow that fails will automatically trigger this handler

### Step 2 — Add success notifications to individual workflows

For each workflow you want to track success on:

1. Open the workflow
2. Add an **HTTP Request** node after your last working node
3. Use the success payload below
4. Connect it as the final step in your flow

### Step 3 — Get your NORA ingest URL

1. In NORA go to your n8n app settings
2. Copy the ingest URL — it looks like `http(s)://your-nora/ingest/{token}`
3. Use this URL in both HTTP Request nodes above

---

## HTTP Request node payload

Use this single payload for both success and error nodes. Fields that aren't
available in a given context will simply send as empty strings.

- **Method:** POST
- **URL:** `http(s)://your-nora/ingest/{token}`
- **Body Content Type:** JSON
- **Body:**

```json
{
  "action": "workflow.success",
  "workflow_name": "={{ $workflow.name }}",
  "workflow_id": "={{ $workflow.id }}",
  "execution_id": "={{ $execution.id }}",
  "error_message": "={{ $execution.lastError?.message ?? '' }}",
  "error_node": "={{ $execution.lastError?.node?.name ?? '' }}"
}
```

For the error handler workflow, change `action` to `workflow.error`. Everything
else stays the same — error fields will populate automatically, and the success
fields will still be present.

---

## Tips

- Use a single shared **Error Workflow** in n8n (Settings → Error Workflow) with the
  error payload above — this catches failures from all workflows automatically.
- The success node needs to be added manually to each workflow you want to track.
- Set **Continue on Fail** to false on the HTTP Request node so errors are visible
  in n8n logs if NORA is unreachable.
