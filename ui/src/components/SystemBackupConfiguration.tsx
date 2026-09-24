import { useRef, useState } from "react";
import { Form } from "react-bootstrap";
import { createSystemBackup, restoreSystemBackup } from "api/settings";
import FileUploader from "components/FileUploader";
import LoadingButton from "components/LoadingButton";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { errorMessage } from "util/response";
import { downloadFile } from "util/util";

const SystemBackupConfiguration = () => {
  const { notify } = useNotification();
  const [withImages, setWithImages] = useState(false);
  const [withUpdates, setWithUpdates] = useState(false);
  const [backupInProgress, setBackupInProgress] = useState(false);
  const [restoreFile, setRestoreFile] = useState<File | null>(null);
  const [restoreInProgress, setRestoreInProgress] = useState(false);
  const resolveRestore = useRef<((result: boolean) => void) | null>(null);

  const onBackup = async () => {
    setBackupInProgress(true);
    try {
      const url = await createSystemBackup(withImages, withUpdates);
      const timestamp = new Date()
        .toISOString()
        .replace(/[-:]/g, "")
        .slice(0, 15);
      downloadFile(url, `operations-center-backup-${timestamp}.tar.gz`);
      notify.success("Backup created");
    } catch (e) {
      notify.error(`Error during backup creation: ${e}`);
    }
    setBackupInProgress(false);
  };

  // The upload is held back, until the restore is confirmed.
  const onRestoreRequested = (file: File | null): Promise<boolean> => {
    if (!file) {
      return Promise.resolve(false);
    }

    setRestoreFile(file);
    return new Promise<boolean>((resolve) => {
      resolveRestore.current = resolve;
    });
  };

  const finishRestore = (result: boolean) => {
    resolveRestore.current?.(result);
    resolveRestore.current = null;
    setRestoreFile(null);
  };

  const onRestoreConfirmed = async () => {
    if (!restoreFile) {
      return;
    }

    setRestoreInProgress(true);
    const result = await restore(restoreFile);
    setRestoreInProgress(false);
    finishRestore(result);
  };

  const restore = async (file: File): Promise<boolean> => {
    try {
      const response = await restoreSystemBackup(file);
      if (response.error_code != 0) {
        notify.error(errorMessage(response));
        return false;
      }

      notify.success(
        "Restore staged, Operations Center is restarting. Reload the page in a moment.",
      );
      return true;
    } catch (e) {
      notify.error(`Error during backup restore: ${e}`);
      return false;
    }
  };

  return (
    <div className="p-3">
      <h5>Backup</h5>
      <p>
        The backup holds the configuration, the certificates and keys and the
        database of Operations Center. It contains secrets and has to be stored
        safely.
      </p>
      <Form.Check
        type="checkbox"
        label="Include images"
        checked={withImages}
        onChange={(e) => setWithImages(e.target.checked)}
      />
      <Form.Check
        type="checkbox"
        className="mb-3"
        label="Include cached update files"
        checked={withUpdates}
        onChange={(e) => setWithUpdates(e.target.checked)}
      />
      <LoadingButton
        isLoading={backupInProgress}
        variant="success"
        size="sm"
        onClick={onBackup}
      >
        Download backup
      </LoadingButton>
      <h5 className="mt-5">Restore</h5>
      <p className="text-danger">
        Restoring a backup replaces the complete state of Operations Center.
        Operations, which have been in progress, when the backup has been
        created, are aborted.
      </p>
      <FileUploader onUpload={onRestoreRequested} />
      <ModalWindow
        show={restoreFile !== null}
        handleClose={() => {
          if (!restoreInProgress) {
            finishRestore(false);
          }
        }}
        title="Restore backup"
        footer={
          <>
            <LoadingButton
              isLoading={restoreInProgress}
              variant="danger"
              onClick={onRestoreConfirmed}
            >
              Restore
            </LoadingButton>
          </>
        }
      >
        <p>
          Are you sure you want to restore the backup "{restoreFile?.name}"?
          This replaces the complete state of Operations Center and restarts it.
        </p>
      </ModalWindow>
    </div>
  );
};

export default SystemBackupConfiguration;
