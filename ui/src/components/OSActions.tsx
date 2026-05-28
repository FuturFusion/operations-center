import type { FC } from "react";
import UpdateCheckBtn from "components/UpdateCheckBtn";
import RebootOSBtn from "components/RebootOSBtn";
import ShutdownOSBtn from "components/ShutdownOSBtn";

const OSActions: FC = () => {
  const items = [
    <UpdateCheckBtn key="update-check" />,
    <RebootOSBtn key="reboot" />,
    <ShutdownOSBtn key="shutdown" />,
  ];

  return (
    <div className="d-flex list-unstyled m-0 p-0 align-items-center action-sep">
      {items.map((item) => (
        <li>{item}</li>
      ))}
    </div>
  );
};

export default OSActions;
