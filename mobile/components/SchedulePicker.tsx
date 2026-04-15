import { useState } from "react";
import { View, Text, Pressable, TextInput } from "react-native";
import { colors } from "@/lib/theme";

// ---------------------------------------------------------------------------
// Schedule modes — matches server's schedule field
// ---------------------------------------------------------------------------

type ScheduleMode = "once" | "interval" | "cron";

const MODES: { key: ScheduleMode; label: string; hint: string }[] = [
  { key: "once", label: "Once", hint: "Run a single time" },
  { key: "interval", label: "Interval", hint: "Repeat every N minutes/hours" },
  { key: "cron", label: "Cron", hint: "Cron expression (e.g. 0 */6 * * *)" },
];

// Preset intervals for quick selection
const INTERVAL_PRESETS = [
  { label: "5m", value: "5m" },
  { label: "15m", value: "15m" },
  { label: "30m", value: "30m" },
  { label: "1h", value: "1h" },
  { label: "6h", value: "6h" },
  { label: "12h", value: "12h" },
  { label: "24h", value: "24h" },
];

interface SchedulePickerProps {
  value: string;
  onChange: (schedule: string) => void;
}

export function SchedulePicker({ value, onChange }: SchedulePickerProps) {
  // Determine current mode from value
  const getMode = (): ScheduleMode => {
    if (!value || value === "once") return "once";
    if (/^\d+[smhd]$/.test(value)) return "interval";
    if (value.includes(" ") || value.includes("*")) return "cron";
    return "interval";
  };

  const [mode, setMode] = useState<ScheduleMode>(getMode);
  const [customInterval, setCustomInterval] = useState(
    mode === "interval" ? value : "",
  );
  const [cronExpr, setCronExpr] = useState(mode === "cron" ? value : "");

  const handleModeChange = (m: ScheduleMode) => {
    setMode(m);
    if (m === "once") {
      onChange("once");
    } else if (m === "interval") {
      const v = customInterval || "5m";
      setCustomInterval(v);
      onChange(v);
    } else {
      const v = cronExpr || "0 */6 * * *";
      setCronExpr(v);
      onChange(v);
    }
  };

  const handlePreset = (preset: string) => {
    setCustomInterval(preset);
    onChange(preset);
  };

  const handleCustomInterval = (text: string) => {
    setCustomInterval(text);
    if (text.trim()) onChange(text.trim());
  };

  const handleCron = (text: string) => {
    setCronExpr(text);
    if (text.trim()) onChange(text.trim());
  };

  return (
    <View>
      {/* Mode selector */}
      <View className="flex-row gap-2 mb-3">
        {MODES.map((m) => {
          const active = mode === m.key;
          return (
            <Pressable
              key={m.key}
              onPress={() => handleModeChange(m.key)}
              className="flex-1 py-2 rounded-lg items-center active:opacity-70"
              style={{
                backgroundColor: active
                  ? `${colors.accent.default}22`
                  : colors.bg.input,
                borderWidth: 1,
                borderColor: active
                  ? colors.accent.default
                  : colors.border.default,
              }}
            >
              <Text
                className="text-xs font-semibold"
                style={{
                  color: active ? colors.accent.default : colors.text.muted,
                }}
              >
                {m.label}
              </Text>
            </Pressable>
          );
        })}
      </View>

      {/* Hint */}
      <Text className="text-text-muted text-xs mb-2">
        {MODES.find((m) => m.key === mode)?.hint}
      </Text>

      {/* Interval presets + custom */}
      {mode === "interval" && (
        <View>
          <View className="flex-row flex-wrap gap-2 mb-2">
            {INTERVAL_PRESETS.map((p) => {
              const active = value === p.value;
              return (
                <Pressable
                  key={p.value}
                  onPress={() => handlePreset(p.value)}
                  className="px-3 py-1.5 rounded-full active:opacity-70"
                  style={{
                    backgroundColor: active
                      ? colors.accent.default
                      : `${colors.accent.default}15`,
                  }}
                >
                  <Text
                    className="text-xs font-semibold"
                    style={{
                      color: active ? "#fff" : colors.accent.hover,
                    }}
                  >
                    {p.label}
                  </Text>
                </Pressable>
              );
            })}
          </View>
          <TextInput
            className="bg-bg-input rounded-lg px-3 py-2.5 text-text-primary text-sm"
            style={{ borderColor: colors.border.default, borderWidth: 1 }}
            placeholder="Custom: 10m, 2h, 1d"
            placeholderTextColor={colors.text.muted}
            value={customInterval}
            onChangeText={handleCustomInterval}
            autoCapitalize="none"
            autoCorrect={false}
          />
        </View>
      )}

      {/* Cron expression */}
      {mode === "cron" && (
        <TextInput
          className="bg-bg-input rounded-lg px-3 py-2.5 text-text-primary text-sm"
          style={{
            borderColor: colors.border.default,
            borderWidth: 1,
            fontFamily: "Courier",
          }}
          placeholder="0 */6 * * *"
          placeholderTextColor={colors.text.muted}
          value={cronExpr}
          onChangeText={handleCron}
          autoCapitalize="none"
          autoCorrect={false}
        />
      )}
    </View>
  );
}
