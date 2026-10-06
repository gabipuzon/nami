import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Nami — codebase map",
  description: "A local map of the dependencies Nami found in your repository.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="en"><body>{children}</body></html>;
}
