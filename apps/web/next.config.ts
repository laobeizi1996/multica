import type { NextConfig } from "next";
import { config } from "dotenv";
import { resolve } from "path";

// Load root .env so REMOTE_API_URL is available to next.config.ts
config({ path: resolve(__dirname, "../../.env") });

const remoteApiUrl = process.env.REMOTE_API_URL || "http://localhost:8080";

const nextConfig: NextConfig = {
  async rewrites() {
    return [
      {
        source: "/api/:path*",
        destination: `${remoteApiUrl}/api/:path*`,
      },
      {
        source: "/ws",
        destination: `${remoteApiUrl}/ws`,
      },
      {
        source: "/auth/:path*",
        destination: `${remoteApiUrl}/auth/:path*`,
      },
    ];
  },
};

export default nextConfig;
