import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Vigía Console",
  description: "Collections compliance supervision console",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="es">
      <body className="bg-slate-50 text-slate-900 antialiased">{children}</body>
    </html>
  );
}
