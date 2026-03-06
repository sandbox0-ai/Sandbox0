import type { Metadata } from "next";
import "@sandbox0/ui/globals.css";

export const metadata: Metadata = {
  title: "Sandbox0 - AI-Native Sandbox Infrastructure",
  description:
    "Persistent storage, session state maintenance, sub-200ms cold starts, and operator-first self-hosted deployment for AI agents.",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" className="dark">
      <body>{children}</body>
    </html>
  );
}
