import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

const devServerHost = "localhost";
const backendServerPort = 45678;

// https://vitejs.dev/config/
export default defineConfig({
	plugins: [react()],
	server: {
		host: "0.0.0.0",
		cors: false,
		open: true,
		proxy: {
			"/oidc-callback": {
				target: `http://${devServerHost}:${backendServerPort}`,
				changeOrigin: true
			},
			"/api": {
				target: `http://${devServerHost}:${backendServerPort}`,
				changeOrigin: true,
				rewrite: (path) => path.replace(/^\/api/, "")
			}
		}
	},
	test: {
		globals: true,
		environment: "jsdom",
		setupFiles: "src/setupTests",
		mockReset: true
	}
});
