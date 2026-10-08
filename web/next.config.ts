import type { NextConfig } from "next";

const nextConfig: NextConfig = process.env.NAMI_EXPORT === "1" ? {
  output: "export",
} : {
  async rewrites() {
    return [{ source: "/api/v1/:path*", destination: "http://127.0.0.1:7331/api/v1/:path*" }];
  },
};

export default nextConfig;
