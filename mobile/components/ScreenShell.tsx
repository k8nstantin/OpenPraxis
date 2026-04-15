import { View, Text } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";

interface Props {
  title: string;
  icon: string;
  subtitle?: string;
  children?: React.ReactNode;
}

export function ScreenShell({ title, icon, subtitle, children }: Props) {
  return (
    <SafeAreaView className="flex-1 bg-bg-primary">
      <View className="flex-1 px-4 pt-4">
        <View className="items-center justify-center py-12">
          <Text className="text-5xl mb-4">{icon}</Text>
          <Text className="text-text-primary text-xl font-semibold">
            {title}
          </Text>
          {subtitle && (
            <Text className="text-text-muted text-sm mt-2 text-center px-8">
              {subtitle}
            </Text>
          )}
        </View>
        {children}
      </View>
    </SafeAreaView>
  );
}
