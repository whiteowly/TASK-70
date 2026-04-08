import { ReactNode } from "react";
import { Navigate } from "react-router-dom";
import { useAuthStore, hasRole } from "../stores/auth";

interface RouteGuardProps {
  role: string;
  children: ReactNode;
}

function RouteGuard({ role, children }: RouteGuardProps) {
  const user = useAuthStore((s) => s.user);
  const loading = useAuthStore((s) => s.loading);

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-gray-50">
        <div className="text-gray-500">Loading...</div>
      </div>
    );
  }

  if (!user) {
    return <Navigate to="/login" replace />;
  }

  if (!hasRole(user, role)) {
    return (
      <div className="flex min-h-screen flex-col items-center justify-center bg-gray-50">
        <h1 className="text-4xl font-bold text-gray-300">403</h1>
        <p className="mt-4 text-lg text-gray-600">
          Forbidden — Insufficient permissions
        </p>
      </div>
    );
  }

  return <>{children}</>;
}

export default RouteGuard;
