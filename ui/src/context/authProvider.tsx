import { FC, ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { fetchSettings } from "api/server";
import { AuthContext } from "context/authContext";

export const AuthProvider: FC<{ children: ReactNode }> = ({ children }) => {
  const {
    data: settings = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["settings"],
    queryFn: fetchSettings,
  });

  return (
    <AuthContext.Provider
      value={{
        isAuthenticated: !!settings?.auth && settings.auth !== "untrusted",
        authMethod: settings?.auth,
        isAuthLoading: isLoading,
        authError: settings ? null : error,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
};
