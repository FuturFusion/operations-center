import { FC, ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { fetchSettings } from "api/server";
import { AuthContext } from "context/authContext";

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
