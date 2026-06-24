import type { FC, ReactNode } from "react";
import { useState } from "react";
import { Button, Form } from "react-bootstrap";
import { useQueryClient } from "@tanstack/react-query";
import FileUploader from "components/FileUploader";
import LoadingButton from "components/LoadingButton";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { downloadFile } from "util/util";
import YAML from "yaml";

export interface OSActionField {
  name: string;
  label: string;
  required?: boolean;
  options?: string[];
  defaultValue?: string;
  type?: "text" | "number" | "checkbox";
}

export type OSActionValues = Record<string, string | boolean>;

export type OSActionInput = object | OSActionValues | File | undefined;

interface Props {
  label: string;
  mode: "confirm" | "data" | "fields" | "download" | "upload";
  run: (input: OSActionInput) => Promise<unknown>;
  successMessage: string;
  variant?: string;
  icon?: ReactNode;
  confirmMessage?: string;
  submitLabel?: string;
  fields?: OSActionField[];
  defaultData?: string;
  filename?: string;
  invalidateKeys?: string[][];
}

const OSAction: FC<Props> = ({
  label,
  mode,
  run,
  successMessage,
  variant = "secondary",
  icon,
  confirmMessage,
  submitLabel,
  fields = [],
  defaultData = "",
  filename = "download",
  invalidateKeys = [],
}) => {
  const queryClient = useQueryClient();
  const { notify } = useNotification();
  const [showModal, setShowModal] = useState(false);
  const [inProgress, setInProgress] = useState(false);
  const [data, setData] = useState(defaultData);
  const [values, setValues] = useState<OSActionValues>(() => {
    const initial: OSActionValues = {};
    fields.forEach((field) => {
      if (field.type === "checkbox") {
        initial[field.name] = false;
      } else if (field.defaultValue !== undefined) {
        initial[field.name] = field.defaultValue;
      }
    });
    return initial;
  });

  const onSuccess = () => {
    notify.success(successMessage);
    invalidateKeys.forEach((queryKey) =>
      queryClient.invalidateQueries({ queryKey }),
    );
  };

  const execute = async (input: OSActionInput): Promise<boolean> => {
    setInProgress(true);
    try {
      await run(input);
      onSuccess();
      setInProgress(false);
      return true;
    } catch (e) {
      notify.error(`${label} failed: ${e}`);
      setInProgress(false);
      return false;
    }
  };

  const handleDownload = async (): Promise<boolean> => {
    setInProgress(true);
    try {
      const url = (await run(undefined)) as string;
      downloadFile(url, filename);
      notify.success(successMessage);
      setInProgress(false);
      return true;
    } catch (e) {
      notify.error(`${label} failed: ${e}`);
      setInProgress(false);
      return false;
    }
  };

  const handleSubmit = async () => {
    if (mode === "download") {
      const downloaded = await handleDownload();
      if (downloaded) {
        setShowModal(false);
      }

      return;
    }

    let input: OSActionInput = undefined;

    if (mode === "data") {
      try {
        input = YAML.parse(data) ?? {};
      } catch (e) {
        notify.error(`Error during YAML value parsing: ${e}`);
        return;
      }
    } else if (mode === "fields") {
      input = values;
    }

    const ok = await execute(input);
    if (ok) {
      setShowModal(false);
    }
  };

  const handleClick = () => {
    setShowModal(true);
  };

  const trigger = icon ? (
    <span
      title={label}
      style={{ color: "grey", cursor: "pointer" }}
      onClick={handleClick}
    >
      {icon}
    </span>
  ) : (
    <Button
      size="sm"
      variant={variant === "danger" ? "danger" : "success"}
      disabled={inProgress}
      onClick={handleClick}
    >
      {label}
    </Button>
  );

  return (
    <>
      {trigger}
      <ModalWindow
        show={showModal}
        scrollable
        handleClose={() => setShowModal(false)}
        title={label}
        footer={
          mode === "upload" ? undefined : (
            <LoadingButton
              isLoading={inProgress}
              variant={variant === "danger" ? "danger" : "success"}
              onClick={handleSubmit}
            >
              {submitLabel ?? label}
            </LoadingButton>
          )
        }
      >
        {(mode === "confirm" || mode === "download") && <p>{confirmMessage}</p>}
        {confirmMessage && (mode === "data" || mode === "fields") && (
          <p className={variant === "danger" ? "text-danger" : ""}>
            {confirmMessage}
          </p>
        )}
        {mode === "data" && (
          <Form.Control
            className="yaml-editor"
            as="textarea"
            rows={10}
            value={data}
            placeholder="key: value"
            onChange={(e) => setData(e.target.value)}
          />
        )}
        {mode === "fields" && (
          <Form>
            {fields.map((field) => (
              <Form.Group key={field.name} className="mb-3">
                {field.type === "checkbox" ? (
                  <Form.Check
                    type="checkbox"
                    label={field.label}
                    checked={Boolean(values[field.name])}
                    onChange={(e) =>
                      setValues({ ...values, [field.name]: e.target.checked })
                    }
                  />
                ) : (
                  <>
                    <Form.Label>{field.label}</Form.Label>
                    {field.options ? (
                      <Form.Select
                        value={String(values[field.name] ?? "")}
                        onChange={(e) =>
                          setValues({ ...values, [field.name]: e.target.value })
                        }
                      >
                        {field.options.map((option) => (
                          <option key={option} value={option}>
                            {option}
                          </option>
                        ))}
                      </Form.Select>
                    ) : (
                      <Form.Control
                        type={field.type ?? "text"}
                        value={String(values[field.name] ?? "")}
                        onChange={(e) =>
                          setValues({ ...values, [field.name]: e.target.value })
                        }
                      />
                    )}
                  </>
                )}
              </Form.Group>
            ))}
          </Form>
        )}
        {mode === "upload" && (
          <FileUploader
            onUpload={async (file) => {
              if (!file) {
                return false;
              }

              const ok = await execute(file);
              if (ok) {
                setShowModal(false);
              }

              return ok;
            }}
          />
        )}
      </ModalWindow>
    </>
  );
};

export default OSAction;
