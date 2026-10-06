import type { NextConfig } from "next";

const backendOrigin = "http://127.0.0.1:7331";

const nextConfig: NextConfig = {
  async rewrites() {
    return [{ source: "/api/v1/:path*", destination: `${backendOrigin}/api/v1/:path*` }];
  },
};

export default nextConfig;
