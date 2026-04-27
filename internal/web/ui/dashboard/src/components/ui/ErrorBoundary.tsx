import { Component, type ErrorInfo, type ReactNode } from 'react';
import { EmptyState } from './EmptyState';
import { Button } from './Button';

interface Props {
  fallback?: (error: Error, reset: () => void) => ReactNode;
  onError?: (error: Error, info: ErrorInfo) => void;
  children: ReactNode;
}

interface State {
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    if (this.props.onError) this.props.onError(error, info);
    else console.error('ErrorBoundary caught', error, info);
  }

  reset = () => this.setState({ error: null });

  render() {
    if (!this.state.error) return this.props.children;
    if (this.props.fallback) return this.props.fallback(this.state.error, this.reset);
    return (
      <EmptyState
        tone="error"
        title="Something went wrong"
        description={this.state.error.message}
        action={<Button onClick={this.reset}>Retry</Button>}
      />
    );
  }
}
