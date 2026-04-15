import { useRef, useEffect, useState, useCallback } from "react";
import { View, Text, ScrollView, Pressable } from "react-native";
import { colors } from "@/lib/theme";
import { PulsingDot } from "@/components/PulsingDot";
import {
  parseTaskOutput,
  type OutputBlock,
  type ParsedOutput,
} from "@/lib/parseOutput";

// ---------------------------------------------------------------------------
// Tool name → color
// ---------------------------------------------------------------------------

const toolColors: Record<string, string> = {
  Bash: "#f59e0b",
  Read: "#3b82f6",
  Write: "#8b5cf6",
  Edit: "#10b981",
  Grep: "#06b6d4",
  Glob: "#6366f1",
  Agent: "#ec4899",
  TodoWrite: "#14b8a6",
  WebSearch: "#f97316",
  WebFetch: "#f97316",
};

function getToolColor(name: string): string {
  return toolColors[name] || colors.accent.default;
}

// ---------------------------------------------------------------------------
// Single block renderers
// ---------------------------------------------------------------------------

function TextBlockRow({ turn, text }: { turn: number; text: string }) {
  return (
    <View
      className="py-2 px-3"
      style={{
        borderBottomWidth: 1,
        borderBottomColor: colors.border.default,
      }}
    >
      <View className="flex-row items-start gap-2">
        <Text
          style={{ color: colors.status.success }}
          className="text-xs font-semibold"
        >
          #{turn}
        </Text>
        <Text
          className="text-xs flex-1"
          style={{ color: colors.text.primary }}
          selectable
        >
          {text}
        </Text>
      </View>
    </View>
  );
}

function ToolUseRow({
  name,
  inputPreview,
}: {
  name: string;
  inputPreview: string;
}) {
  const badgeColor = getToolColor(name);
  return (
    <View
      className="py-1.5 px-3 flex-row items-center gap-2"
      style={{
        borderBottomWidth: 1,
        borderBottomColor: colors.border.default,
      }}
    >
      <View
        className="px-1.5 py-0.5 rounded"
        style={{ backgroundColor: `${badgeColor}22` }}
      >
        <Text
          style={{ color: badgeColor, fontSize: 10, fontWeight: "600" }}
        >
          {name}
        </Text>
      </View>
      <Text
        className="text-xs flex-1"
        style={{
          color: colors.text.secondary,
          fontFamily: "Menlo",
          fontSize: 11,
        }}
        numberOfLines={2}
        selectable
      >
        {inputPreview}
      </Text>
    </View>
  );
}

function ToolResultRow({
  isError,
  preview,
}: {
  isError: boolean;
  preview: string;
}) {
  if (!preview) return null;
  return (
    <View
      className="py-1.5 px-3 pl-6"
      style={{
        borderBottomWidth: 1,
        borderBottomColor: colors.border.default,
        backgroundColor: isError ? `${colors.status.error}08` : undefined,
      }}
    >
      <Text
        className="text-xs"
        style={{
          color: isError ? colors.status.error : colors.text.muted,
          fontFamily: "Menlo",
          fontSize: 10,
        }}
        numberOfLines={4}
        selectable
      >
        {preview}
      </Text>
    </View>
  );
}

function ResultRow({
  terminalReason,
  numTurns,
  costUsd,
}: {
  terminalReason: string;
  numTurns: number;
  costUsd: number;
}) {
  const reasonColor =
    terminalReason === "completed"
      ? colors.status.success
      : terminalReason === "max_turns"
        ? colors.status.warning
        : colors.status.error;

  return (
    <View
      className="py-2.5 px-3 flex-row items-center gap-3"
      style={{
        borderTopWidth: 2,
        borderTopColor: colors.border.default,
        marginTop: 2,
      }}
    >
      <Text style={{ color: reasonColor, fontSize: 12, fontWeight: "600" }}>
        {terminalReason}
      </Text>
      <Text style={{ color: colors.text.muted, fontSize: 12 }}>
        {numTurns} turns
      </Text>
      {costUsd > 0 && (
        <Text style={{ color: colors.text.muted, fontSize: 12 }}>
          ${costUsd.toFixed(2)}
        </Text>
      )}
    </View>
  );
}

// ---------------------------------------------------------------------------
// Block renderer dispatcher
// ---------------------------------------------------------------------------

function OutputBlockView({ block }: { block: OutputBlock }) {
  switch (block.type) {
    case "text":
      return <TextBlockRow turn={block.turn} text={block.text} />;
    case "tool_use":
      return <ToolUseRow name={block.name} inputPreview={block.inputPreview} />;
    case "tool_result":
      return (
        <ToolResultRow isError={block.isError} preview={block.preview} />
      );
    case "result":
      return (
        <ResultRow
          terminalReason={block.terminalReason}
          numTurns={block.numTurns}
          costUsd={block.costUsd}
        />
      );
  }
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

interface TaskOutputProps {
  lines: string[];
  running: boolean;
}

export function TaskOutput({ lines, running }: TaskOutputProps) {
  const scrollRef = useRef<ScrollView>(null);
  const [follow, setFollow] = useState(true);
  const [showResults, setShowResults] = useState(false);
  const prevBlockCount = useRef(0);

  const parsed: ParsedOutput = parseTaskOutput(lines);

  // Separate tool_result blocks for collapsible toggle
  const displayBlocks = showResults
    ? parsed.blocks
    : parsed.blocks.filter((b) => b.type !== "tool_result");

  // Auto-scroll when new blocks arrive and follow mode is on
  useEffect(() => {
    if (follow && parsed.blocks.length > prevBlockCount.current) {
      setTimeout(() => {
        scrollRef.current?.scrollToEnd({ animated: true });
      }, 100);
    }
    prevBlockCount.current = parsed.blocks.length;
  }, [parsed.blocks.length, follow]);

  const handleScroll = useCallback(
    (e: { nativeEvent: { contentOffset: { y: number }; contentSize: { height: number }; layoutMeasurement: { height: number } } }) => {
      const { contentOffset, contentSize, layoutMeasurement } = e.nativeEvent;
      const distanceFromBottom =
        contentSize.height - layoutMeasurement.height - contentOffset.y;
      // If user scrolls up more than 50px, disable follow
      setFollow(distanceFromBottom < 50);
    },
    [],
  );

  if (parsed.blocks.length === 0) {
    return (
      <View className="bg-bg-card rounded-card p-4 items-center">
        {running ? (
          <View className="flex-row items-center gap-2">
            <PulsingDot color={colors.status.success} />
            <Text className="text-text-muted text-xs">
              Waiting for output...
            </Text>
          </View>
        ) : (
          <Text className="text-text-muted text-xs">No output</Text>
        )}
      </View>
    );
  }

  const toolUseCount = parsed.blocks.filter(
    (b) => b.type === "tool_use",
  ).length;
  const resultCount = parsed.blocks.filter(
    (b) => b.type === "tool_result",
  ).length;

  return (
    <View>
      {/* Controls bar */}
      <View className="flex-row items-center justify-between mb-2">
        <View className="flex-row items-center gap-3">
          {running && (
            <View className="flex-row items-center gap-1.5">
              <PulsingDot color={colors.status.success} />
              <Text
                className="text-xs font-medium"
                style={{ color: colors.status.success }}
              >
                Live
              </Text>
            </View>
          )}
          <Text className="text-text-muted text-xs">
            {parsed.totalLines} lines \u00B7 {toolUseCount} tools
          </Text>
        </View>
        <View className="flex-row items-center gap-3">
          {resultCount > 0 && (
            <Pressable onPress={() => setShowResults(!showResults)}>
              <Text
                className="text-xs"
                style={{ color: colors.accent.default }}
              >
                {showResults ? "Hide results" : "Show results"}
              </Text>
            </Pressable>
          )}
          {running && (
            <Pressable onPress={() => setFollow(!follow)}>
              <Text
                className="text-xs"
                style={{
                  color: follow
                    ? colors.status.success
                    : colors.text.muted,
                }}
              >
                {follow ? "\u25BC Follow" : "\u25BC Scroll"}
              </Text>
            </Pressable>
          )}
        </View>
      </View>

      {/* Output blocks */}
      <View
        className="bg-bg-card rounded-card overflow-hidden"
        style={{ maxHeight: 400 }}
      >
        <ScrollView
          ref={scrollRef}
          onScroll={handleScroll}
          scrollEventThrottle={16}
        >
          {displayBlocks.map((block, i) => (
            <OutputBlockView key={i} block={block} />
          ))}
        </ScrollView>
      </View>
    </View>
  );
}
