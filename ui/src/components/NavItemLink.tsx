import { FC } from "react";
import { Nav } from "react-bootstrap";
import { Link } from "react-router";

export interface MenuItem {
  id: string;
  to?: string;
  menu: string[];
}

export interface NavItemLinkProps
  extends React.ComponentProps<typeof Nav.Link> {
  item?: MenuItem;
  isActive?: boolean;
  children: React.ReactNode;
}

export const NavItemLink: FC<NavItemLinkProps> = ({
  item,
  isActive,
  children,
  ...navLinkProps
}) => {
  return (
    <li className={isActive ? "nav-link-active" : ""}>
      <Nav.Link as={Link} to={item?.to} {...navLinkProps}>
        {children}
      </Nav.Link>
    </li>
  );
};
