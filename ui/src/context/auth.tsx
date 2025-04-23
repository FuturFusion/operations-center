import { createContext, FC, ReactNode, useContext } from "react";
import { useQuery } from "@tanstack/react-query";
import { fetchSettings } from "api/server";

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

export const AuthProvider: FC<{ children: ReactNode }> = ({ children }) => {
  const { data: settings = null, isLoading } = useQuery({
    queryKey: ["settings"],
    queryFn: fetchSettings,
  });

  return (
    <AuthContext.Provider
      value={{
        isAuthenticated: (settings && settings.auth !== "untrusted") ?? false,
        authMethod: settings?.auth,
        isAuthLoading: isLoading,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
};

export function useAuth() {
  return useContext(AuthContext);
}
