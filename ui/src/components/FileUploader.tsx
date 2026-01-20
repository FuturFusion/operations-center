import { FC, useRef, useState } from "react";
import { Button, Form, InputGroup, Spinner } from "react-bootstrap";

interface Props {
  onUpload: (file: File | null) => Promise<boolean>;
}

const FileUploader: FC<Props> = ({ onUpload }) => {
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const [file, setFile] = useState<File | null>(null);
  const [uploadInProgress, setUploadInProgress] = useState(false);

  const clearFile = () => {
    if (fileInputRef.current) {
      fileInputRef.current.value = "";
    }

    setFile(null);
  };

  const handleFileChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    if (event.target.files && event.target.files.length > 0) {
      setFile(event.target.files[0]);
    }
  };

  const handleUpload = async () => {
    if (!file || uploadInProgress) return;

    setUploadInProgress(true);
    const result = await onUpload(file);
    if (result) {
      clearFile();
    }

    setUploadInProgress(false);
  };

  return (
    <InputGroup>
      <Form.Control
        type="file"
        size="sm"
        style={{ maxWidth: "300px" }}
        ref={fileInputRef}
        onChange={handleFileChange}
      />
      <Button
        onClick={handleUpload}
        disabled={!file}
        variant="success"
        size="sm"
      >
        {uploadInProgress ? (
          <Spinner
            animation="border"
            role="status"
            variant="outline-secondary"
            style={{ width: "1rem", height: "1rem" }}
          />
        ) : (
          "Upload"
        )}
      </Button>
    </InputGroup>
  );
};

export default FileUploader;
