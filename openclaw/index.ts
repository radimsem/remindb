import { definePluginEntry } from "openclaw/plugin-sdk/plugin-entry";

export default definePluginEntry({
    id: "remindb",
    name: "remindb",
    description:
        "Mounts the remindb MCP server so OpenClaw agents can query and update a compiled view of their workspace.",
    register(_api) {
        // Tools are contributed via .mcp.json
    },
});
