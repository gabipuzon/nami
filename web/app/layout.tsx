import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "nami — codebase map",
  description: "A local map of the dependencies nami found in your repository.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="en"><body>{children}</body></html>;
}
