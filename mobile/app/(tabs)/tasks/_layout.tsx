import { Stack } from "expo-router";
import { colors } from "@/lib/theme";

export default function TasksLayout() {
  return (
    <Stack
      screenOptions={{
        headerStyle: { backgroundColor: colors.bg.secondary },
        headerTintColor: colors.text.primary,
        headerTitleStyle: { fontWeight: "600" },
        contentStyle: { backgroundColor: colors.bg.primary },
      }}
    >
      <Stack.Screen name="index" options={{ title: "Tasks" }} />
      <Stack.Screen name="[id]" options={{ title: "Task" }} />
      <Stack.Screen name="create" options={{ title: "New Task", presentation: "modal" }} />
    </Stack>
  );
}
