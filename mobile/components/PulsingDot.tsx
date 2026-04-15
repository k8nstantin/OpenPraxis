import { useEffect } from "react";
import Animated, {
  useSharedValue,
  useAnimatedStyle,
  withRepeat,
  withTiming,
  Easing,
} from "react-native-reanimated";

interface Props {
  color: string;
  size?: number;
}

export function PulsingDot({ color, size = 8 }: Props) {
  const opacity = useSharedValue(1);

  useEffect(() => {
    opacity.value = withRepeat(
      withTiming(0.3, { duration: 1000, easing: Easing.inOut(Easing.ease) }),
      -1,
      true,
    );
  }, [opacity]);

  const style = useAnimatedStyle(() => ({
    width: size,
    height: size,
    borderRadius: size / 2,
    backgroundColor: color,
    opacity: opacity.value,
  }));

  return <Animated.View style={style} />;
}
