"use client";

import { create } from "zustand";
import { persist } from "zustand/middleware";

interface JwtPayload {
  user_id: string;
  tenant_id: string;
  role: string;
  is_super_admin: boolean;
  exp: number;
  iat: number;
}

interface AuthState {
  token: string | null;
  tenantId: string | null;
  userId: string | null;
  setAuth: (token: string, tenantId: string, userId: string) => void;
  clear: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      tenantId: null,
      userId: null,
      setAuth: (token, tenantId, userId) => set({ token, tenantId, userId }),
      clear: () => set({ token: null, tenantId: null, userId: null }),
    }),
    { name: "auth-storage" }
  )
);

export function getToken(): string | null {
  return useAuthStore.getState().token;
}

export function getTenantId(): string | null {
  return useAuthStore.getState().tenantId;
}

export function clearToken(): void {
  useAuthStore.getState().clear();
}

export function setToken(token: string, tenantId: string): void {
  const payload = decodeToken(token);
  useAuthStore.getState().setAuth(token, tenantId, payload?.user_id ?? "");
}

export function decodeToken(token: string): JwtPayload | null {
  const parts = token.split(".");
  if (parts.length !== 3) return null;
  try {
    const json = atob(parts[1].replace(/-/g, "+").replace(/_/g, "/"));
    return JSON.parse(json) as JwtPayload;
  } catch {
    return null;
  }
}

export function isTokenExpired(token: string): boolean {
  const payload = decodeToken(token);
  if (!payload || !payload.exp) return true;
  return Date.now() >= payload.exp * 1000 - 30_000;
}

export function isAuthenticated(): boolean {
  const token = getToken();
  if (!token) return false;
  return !isTokenExpired(token);
}

export function logout(): void {
  useAuthStore.getState().clear();
  if (typeof window !== "undefined" && window.location.pathname !== "/") {
    window.location.href = "/";
  }
}
