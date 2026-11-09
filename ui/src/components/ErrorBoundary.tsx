import { Component, ErrorInfo, ReactNode } from "react";

interface Props {
  label: string;
  children: ReactNode;
}

interface State {
  error: Error | null;
}

// ErrorBoundary shows an error message instead of its children, if rendering
// them throws. Without it, React unmounts the whole page and leaves it blank.
class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error(`Error while rendering ${this.props.label}`, error, info);
  }

  render() {
    if (this.state.error) {
      return (
        <div>
          Error while rendering {this.props.label}: {this.state.error.message}
        </div>
      );
    }

    return this.props.children;
  }
}

export default ErrorBoundary;
