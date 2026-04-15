import { View, Text } from "react-native";
import { colors } from "@/lib/theme";

type Status = "running" | "success" | "warning" | "error" | "idle";

const statusColors: Record<Status, string> = {
  running: colors.status.success,
  success: colors.status.success,
  warning: colors.status.warning,
  error: colors.status.error,
  idle: colors.text.muted,
};

export function StatusBadge({
  status,
  label,
}: {
  status: Status;
  label: string;
}) {
  const color = statusColors[status];
  return (
    <View className="flex-row items-center gap-1.5">
      <View
        style={{ backgroundColor: color, width: 8, height: 8, borderRadius: 4 }}
      />
      <Text style={{ color }} className="text-xs font-medium">
        {label}
      </Text>
    </View>
  );
}
