import { createContext, useContext } from "react";

interface ContextProps {
  isAuthenticated: boolean;
  isAuthLoading: boolean;
  authMethod: string | undefined;
}

const initialState: ContextProps = {
  isAuthenticated: false,
  isAuthLoading: true,
  authMethod: "",
};

export const AuthContext = createContext<ContextProps>(initialState);

export function useAuth() {
  return useContext(AuthContext);
}
