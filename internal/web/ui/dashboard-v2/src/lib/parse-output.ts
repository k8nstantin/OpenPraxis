// Parse Claude stream-json task output into typed blocks for the Live
// Output tab. Each line emitted from `/api/tasks/{id}/output` is one
// JSON event:
//   { type: 'assistant', message: { content: [{ type: 'text' | 'tool_use', ... }] } }
//   { type: 'user', message: { content: [{ type: 'tool_result', ... }] } }
//   { type: 'result', terminal_reason, num_turns, total_cost_usd }
//
// Ported from mobile/lib/parseOutput.ts. Kept lossy-but-cheap: text and
// tool inputs are truncated for display; the raw blob is still on the
// run record for callers that want everything.

export type OutputBlockType = 'text' | 'tool_use' | 'tool_result' | 'result'

export interface TextBlock {
  type: 'text'
  turn: number
  text: string
}

export interface ToolUseBlock {
  type: 'tool_use'
  turn: number
  toolId: string
  name: string
  inputPreview: string
  inputRaw: Record<string, unknown>
}

export interface ToolResultBlock {
  type: 'tool_result'
  toolUseId: string
  isError: boolean
  preview: string
}

export interface ResultBlock {
  type: 'result'
  terminalReason: string
  numTurns: number
  costUsd: number
}

export type OutputBlock =
  | TextBlock
  | ToolUseBlock
  | ToolResultBlock
  | ResultBlock

export interface ParsedOutput {
  blocks: OutputBlock[]
  totalLines: number
}

function extractInputPreview(input: Record<string, unknown>): string {
  if (!input) return ''
  if (typeof input.command === 'string') return input.command
  if (typeof input.file_path === 'string') return input.file_path
  if (typeof input.pattern === 'string') return input.pattern
  if (typeof input.query === 'string') return input.query
  if (typeof input.content === 'string') return input.content.slice(0, 100)
  if (typeof input.prompt === 'string') return input.prompt.slice(0, 100)
  if (typeof input.description === 'string')
    return input.description.slice(0, 100)
  const s = JSON.stringify(input)
  return s.length > 120 ? s.slice(0, 120) + '…' : s
}

function truncate(s: string, max: number): string {
  return s.length > max ? s.slice(0, max) + '…' : s
}

export function parseTaskOutput(lines: string[]): ParsedOutput {
  if (!lines || lines.length === 0) return { blocks: [], totalLines: 0 }
  const totalLines = lines.length
  const blocks: OutputBlock[] = []
  let turnNum = 0

  for (const line of lines) {
    if (!line.trim()) continue
    try {
      const event = JSON.parse(line)

      if (event.type === 'assistant' && event.message) {
        turnNum++
        const content = event.message.content || []
        for (const block of content) {
          if (block.type === 'text' && block.text) {
            blocks.push({
              type: 'text',
              turn: turnNum,
              text: block.text,
            })
          }
          if (block.type === 'tool_use') {
            const inputRaw =
              (block.input as Record<string, unknown>) ?? {}
            blocks.push({
              type: 'tool_use',
              turn: turnNum,
              toolId: block.id || '',
              name: block.name || '?',
              inputPreview: truncate(extractInputPreview(inputRaw), 100),
              inputRaw,
            })
          }
        }
      }

      if (event.type === 'user' && event.message) {
        const content = event.message.content || []
        for (const block of content) {
          if (block.type === 'tool_result') {
            let preview = ''
            if (typeof block.content === 'string') {
              preview = block.content
            } else if (Array.isArray(block.content)) {
              preview = block.content
                .filter((c: { type: string }) => c.type === 'text')
                .map((c: { text: string }) => c.text)
                .join('\n')
            }
            blocks.push({
              type: 'tool_result',
              toolUseId: block.tool_use_id || '',
              isError: block.is_error === true,
              preview,
            })
          }
        }
      }

      if (event.type === 'result') {
        blocks.push({
          type: 'result',
          terminalReason:
            event.terminal_reason || event.stop_reason || 'unknown',
          numTurns: event.num_turns || 0,
          costUsd: event.total_cost_usd || 0,
        })
      }
    } catch {
      // Skip unparseable lines
    }
  }

  return { blocks, totalLines }
}
