import { View, Text, Pressable } from "react-native";
import { useRouter } from "expo-router";
import { colors } from "@/lib/theme";
import { PulsingDot } from "./PulsingDot";
import type { Task } from "@/lib/types";

interface Props {
  tasks: Task[];
  onKill: (id: string) => void;
  killing?: string | null;
}

function RunningTaskRow({
  task,
  onKill,
  killing,
}: {
  task: Task;
  onKill: (id: string) => void;
  killing?: string | null;
}) {
  const router = useRouter();
  const isKilling = killing === task.id;

  return (
    <Pressable
      className="flex-row items-center py-3 active:opacity-70"
      onPress={() => router.push(`/tasks/${task.id}`)}
      style={{ borderBottomWidth: 1, borderBottomColor: colors.border.default }}
    >
      <PulsingDot color={colors.status.success} size={10} />
      <View className="flex-1 ml-3">
        <Text className="text-text-primary text-sm font-medium" numberOfLines={1}>
          {task.title}
        </Text>
        <Text className="text-text-muted text-xs mt-0.5" numberOfLines={1}>
          {task.marker} {task.agent ? `\u00B7 ${task.agent}` : ""}
        </Text>
      </View>
      <Pressable
        className="px-3 py-1.5 rounded-md active:opacity-70"
        style={{ backgroundColor: `${colors.status.error}33` }}
        onPress={(e) => {
          e.stopPropagation();
          onKill(task.id);
        }}
        disabled={isKilling}
      >
        <Text
          style={{ color: colors.status.error }}
          className="text-xs font-semibold"
        >
          {isKilling ? "Killing..." : "Stop"}
        </Text>
      </Pressable>
    </Pressable>
  );
}

export function RunningTasks({ tasks, onKill, killing }: Props) {
  if (tasks.length === 0) {
    return (
      <View className="bg-bg-card rounded-card p-6 items-center">
        <Text className="text-text-muted text-sm">No running tasks</Text>
      </View>
    );
  }

  return (
    <View className="bg-bg-card rounded-card px-4">
      {tasks.map((task) => (
        <RunningTaskRow
          key={task.id}
          task={task}
          onKill={onKill}
          killing={killing}
        />
      ))}
    </View>
  );
}
