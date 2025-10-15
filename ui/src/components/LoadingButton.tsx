import { FC } from "react";
import { Button, Spinner, ButtonProps } from "react-bootstrap";

interface LoadingButtonProps extends ButtonProps {
  isLoading?: boolean;
}

const LoadingButton: FC<LoadingButtonProps> = ({
  isLoading,
  children,
  disabled,
  ...props
}) => {
  return (
    <Button {...props} disabled={isLoading || disabled}>
      {isLoading ? (
        <>
          <Spinner
            as="span"
            animation="border"
            size="sm"
            role="status"
            aria-hidden="true"
          />{" "}
          Loading...
        </>
      ) : (
        children
      )}
    </Button>
  );
};

export default LoadingButton;
