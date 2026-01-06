import { FC } from "react";
import { Form } from "react-bootstrap";
import { BootSecurity } from "util/token";
import { BootSecurityType } from "types/token";

interface Props {
  value: BootSecurityType;
  onChange: (value: string) => void;
}

const BootSecuritySelect: FC<Props> = ({ value, onChange }) => {
  return (
    <Form.Group className="mb-4" controlId="boot_security">
      <Form.Label>Boot security</Form.Label>
      <div className="form-text text-muted mb-3">
        IncusOS relies on both UEFI Secure Boot and a TPM 2.0 module to provide
        strong boot security. Unfortunately some systems don't fully support
        those features. To handle that, IncusOS can be installed with degraded
        boot security.
      </div>
      <Form.Check
        type="radio"
        label="Optimal boot security (UEFI Secure Boot & TPM 2.0)"
        name="boot_security"
        checked={value == BootSecurity.OPTIMAL}
        onChange={() => onChange(BootSecurity.OPTIMAL)}
      />
      <Form.Check
        type="radio"
        label="Degraded boot security (no TPM 2.0 module)"
        name="boot_security"
        checked={value == BootSecurity.NO_TPM}
        onChange={() => onChange(BootSecurity.NO_TPM)}
      />
      <Form.Check
        type="radio"
        label="Degraded boot security (no Secure Boot)"
        name="boot_security"
        checked={value == BootSecurity.NO_SECURE_BOOT}
        onChange={() => onChange(BootSecurity.NO_SECURE_BOOT)}
      />
    </Form.Group>
  );
};

export default BootSecuritySelect;
