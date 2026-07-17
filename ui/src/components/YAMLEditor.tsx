import type { FC, KeyboardEvent } from "react";
import { useEffect, useState } from "react";
import { Form } from "react-bootstrap";
import LoadingButton from "components/LoadingButton";

interface Props {
  yamlData: string;
  onSubmit: (value: string) => Promise<boolean>;
}

const YamlEditor: FC<Props> = ({ yamlData, onSubmit }) => {
  const [isEditing, setIsEditing] = useState(false);
  const [isSubmiting, setIsSubmiting] = useState(false);
  const [yaml, setYaml] = useState(yamlData);

  useEffect(() => {
    setYaml(yamlData);
  }, [yamlData]);

  const submitForm = async (value: string) => {
    setIsSubmiting(true);
    const result = await onSubmit(value);
    if (result) {
      setIsEditing(false);
    }

    setIsSubmiting(false);
  };

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Tab") {
      e.preventDefault();
      const target = e.target as HTMLTextAreaElement;
      const start = target.selectionStart;
      const end = target.selectionEnd;

      const newValue = yaml.substring(0, start) + "  " + yaml.substring(end);

      setYaml(newValue);

      setTimeout(() => {
        target.selectionStart = target.selectionEnd = start + 2;
      }, 0);
    }
  };

  return (
    <div style={{ width: "100%", height: "100%" }}>
      {isEditing ? (
        <Form.Control
          className="yaml-editor"
          as="textarea"
          value={yaml}
          onChange={(e) => setYaml(e.target.value)}
          onKeyDown={handleKeyDown}
        />
      ) : (
        <pre className="bg-body-tertiary border rounded-3 p-3 mb-0 yaml-editor">
          {yaml}
        </pre>
      )}
      <div className="d-flex justify-content-between align-items-center mt-4">
        <Form.Check
          type="switch"
          label="Edit mode"
          id="yaml-mode-switch"
          checked={isEditing}
          onChange={() => setIsEditing(!isEditing)}
        />
        {isEditing && (
          <LoadingButton
            isLoading={isSubmiting}
            className="mt-3 float-end"
            variant="success"
            onClick={() => submitForm(yaml)}
          >
            Submit
          </LoadingButton>
        )}
      </div>
    </div>
  );
};

export default YamlEditor;
