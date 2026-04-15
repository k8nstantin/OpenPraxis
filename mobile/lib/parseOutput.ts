/**
 * Parse Claude stream-json output lines into structured blocks for display.
 *
 * Each line from /api/tasks/{id}/output is a JSON event:
 *   { type: "assistant", message: { content: [...] } }
 *   { type: "user", message: { content: [...] } }  (tool_result)
 *   { type: "result", terminal_reason, num_turns, total_cost_usd }
 */

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type OutputBlockType = "text" | "tool_use" | "tool_result" | "result";

export interface TextBlock {
  type: "text";
  turn: number;
  text: string;
}

export interface ToolUseBlock {
  type: "tool_use";
  turn: number;
  toolId: string;
  name: string;
  inputPreview: string;
}

export interface ToolResultBlock {
  type: "tool_result";
  toolUseId: string;
  isError: boolean;
  preview: string;
}

export interface ResultBlock {
  type: "result";
  terminalReason: string;
  numTurns: number;
  costUsd: number;
}

export type OutputBlock = TextBlock | ToolUseBlock | ToolResultBlock | ResultBlock;

export interface ParsedOutput {
  blocks: OutputBlock[];
  totalLines: number;
}

// ---------------------------------------------------------------------------
// Parser
// ---------------------------------------------------------------------------

function extractInputPreview(input: Record<string, unknown>): string {
  if (!input) return "";
  // Pick the most informative field
  if (typeof input.command === "string") return input.command;
  if (typeof input.file_path === "string") return input.file_path;
  if (typeof input.pattern === "string") return input.pattern;
  if (typeof input.query === "string") return input.query;
  if (typeof input.content === "string")
    return input.content.substring(0, 100);
  if (typeof input.prompt === "string") return input.prompt.substring(0, 100);
  if (typeof input.description === "string")
    return input.description.substring(0, 100);
  // Fallback: stringify first 120 chars
  const s = JSON.stringify(input);
  return s.length > 120 ? s.substring(0, 120) + "..." : s;
}

function truncate(s: string, max: number): string {
  return s.length > max ? s.substring(0, max) + "..." : s;
}

export function parseTaskOutput(lines: string[]): ParsedOutput {
  if (!lines || lines.length === 0) {
    return { blocks: [], totalLines: 0 };
  }

  const totalLines = lines.length;
  // For very large outputs, only parse the last 500 lines
  const toProcess = totalLines > 500 ? lines.slice(-500) : lines;

  const blocks: OutputBlock[] = [];
  let turnNum = 0;

  for (const line of toProcess) {
    if (!line.trim()) continue;
    try {
      const event = JSON.parse(line);

      if (event.type === "assistant" && event.message) {
        turnNum++;
        const content = event.message.content || [];
        for (const block of content) {
          if (block.type === "text" && block.text) {
            blocks.push({
              type: "text",
              turn: turnNum,
              text: truncate(block.text, 500),
            });
          }
          if (block.type === "tool_use") {
            blocks.push({
              type: "tool_use",
              turn: turnNum,
              toolId: block.id || "",
              name: block.name || "?",
              inputPreview: truncate(
                extractInputPreview(block.input),
                100,
              ),
            });
          }
        }
      }

      if (event.type === "user" && event.message) {
        const content = event.message.content || [];
        for (const block of content) {
          if (block.type === "tool_result") {
            let preview = "";
            if (typeof block.content === "string") {
              preview = block.content;
            } else if (Array.isArray(block.content)) {
              const textParts = block.content
                .filter((c: { type: string }) => c.type === "text")
                .map((c: { text: string }) => c.text);
              preview = textParts.join("\n");
            }
            blocks.push({
              type: "tool_result",
              toolUseId: block.tool_use_id || "",
              isError: block.is_error === true,
              preview: truncate(preview, 300),
            });
          }
        }
      }

      if (event.type === "result") {
        blocks.push({
          type: "result",
          terminalReason:
            event.terminal_reason || event.stop_reason || "unknown",
          numTurns: event.num_turns || 0,
          costUsd: event.total_cost_usd || 0,
        });
      }
    } catch {
      // Skip unparseable lines
    }
  }

  return { blocks, totalLines };
}
