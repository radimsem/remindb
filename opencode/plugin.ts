import type { Plugin } from "@opencode-ai/plugin";

export const RemindbPlugin: Plugin = async (_ctx) => {
    // The MCP server is mounted via opencode.json -> mcp.remindb.
    return {};
};

export default RemindbPlugin;
