import type { FC, ReactNode } from "react";

interface Props {
  groups: ReactNode[][];
}

// Renders one or more clusters of action buttons, separated like the primary
// OS actions (a "|" between items in a cluster, a gap between clusters).
const ActionGroup: FC<Props> = ({ groups }) => (
  <div className="d-flex align-items-center gap-4">
    {groups.map((items, g) => (
      <ul
        key={g}
        className="d-flex list-unstyled m-0 p-0 align-items-center action-sep"
      >
        {items.map((item, i) => (
          <li key={i}>{item}</li>
        ))}
      </ul>
    ))}
  </div>
);

export default ActionGroup;
