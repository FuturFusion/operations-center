import { FC } from "react";
import { Form } from "react-bootstrap";
import { FormikProps } from "formik/dist/types";
import ArchSelect from "components/ArchSelect";
import ImageTypeSelect from "components/ImageTypeSelect";
import { TokenSeedImageFormValues } from "types/token";

interface Props {
  formik: FormikProps<TokenSeedImageFormValues>;
}

const TokenSeedImageForm: FC<Props> = ({ formik }) => {
  return (
    <div>
      <Form noValidate>
        <ImageTypeSelect
          value={formik.values.type}
          onChange={(val) => formik.setFieldValue("type", val)}
        />
        <ArchSelect
          value={formik.values.architecture}
          onChange={(val) => formik.setFieldValue("architecture", val)}
        />
      </Form>
    </div>
  );
};

export default TokenSeedImageForm;
