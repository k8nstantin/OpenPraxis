import { Component, type ErrorInfo, type ReactNode } from 'react';

interface Props {
  fallback?: (error: Error, reset: () => void) => ReactNode;
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
    console.error('ErrorBoundary caught', error, info.componentStack);
  }

  reset = () => this.setState({ error: null });

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;
    if (this.props.fallback) return this.props.fallback(error, this.reset);
    return (
      <div className="ui-empty ui-empty--error" role="alert">
        <div className="ui-empty__title">Something went wrong</div>
        <div className="ui-empty__message">{error.message}</div>
        <div className="ui-empty__action">
          <button type="button" className="ui-btn ui-btn--secondary ui-btn--md" onClick={this.reset}>
            Try again
          </button>
        </div>
      </div>
    );
  }
}
