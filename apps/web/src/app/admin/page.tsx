"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export default function AdminHome() {
  const [token, setToken] = useState("");
  const [msg, setMsg] = useState("");

  async function login() {
    setMsg("");
    const res = await fetch("/admin/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token }),
    });
    if (!res.ok) {
      setMsg("Invalid token.");
      return;
    }
    setMsg("Logged in. Go to /admin/db");
  }

  return (
    <main className="mx-auto max-w-lg p-6">
      <Card>
        <CardHeader>
          <CardTitle>Admin Login</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <Input
            placeholder="ADMIN_TOKEN"
            value={token}
            onChange={(e) => setToken(e.target.value)}
          />
          <Button onClick={login}>Login</Button>
          {msg && <p className="text-sm text-muted-foreground">{msg}</p>}
          <p className="text-sm text-muted-foreground">
            After login, open <code>/admin/db</code>.
          </p>
        </CardContent>
      </Card>
    </main>
  );
}
