import type { NextConfig } from "next";

const eventServiceUrl = process.env.EVENT_SERVICE_URL ?? "http://localhost:8081";

const nextConfig: NextConfig = {
  async rewrites() {
    return [
      {
        source: "/api/events/:path*",
        destination: `${eventServiceUrl}/api/v1/events/:path*`,
      },
    ];
  },
};

export default nextConfig;
