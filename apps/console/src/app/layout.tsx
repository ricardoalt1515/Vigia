import type { Metadata } from "next";
import Link from "next/link";
import "./globals.css";

export const metadata: Metadata = {
  title: "Vigía Console",
  description: "Collections compliance supervision console",
};

function NavBar() {
  return (
    <nav className="border-b border-slate-200 bg-white px-8 py-3">
      <div className="flex gap-6 text-sm font-medium text-slate-600">
        <Link href="/interactions" className="hover:text-slate-900">
          Interactions
        </Link>
        <Link href="/dashboards/by-despacho" className="hover:text-slate-900">
          By Despacho
        </Link>
        <Link href="/dashboards/by-cause" className="hover:text-slate-900">
          By Cause
        </Link>
        <Link href="/dashboards/cost-quality" className="hover:text-slate-900">
          Cost & Quality
        </Link>
      </div>
    </nav>
  );
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="es">
      <body className="bg-slate-50 text-slate-900 antialiased">
        <NavBar />
        {children}
      </body>
    </html>
  );
}
