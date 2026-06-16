/** @type {import('next').NextConfig} */
const nextConfig = {
  async rewrites() {
    return [
      { source: '/api/xeme/:path*', destination: 'http://localhost:8088/v1/:path*' },
      { source: '/api/health', destination: 'http://localhost:8088/health' },
    ];
  },
};
export default nextConfig;
