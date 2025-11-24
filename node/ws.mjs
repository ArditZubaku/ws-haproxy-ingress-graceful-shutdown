import { execSync } from "node:child_process";
import WebSocket from 'ws';

// Parse command line arguments
const args = process.argv.slice(2);
const useSlowEndpoint = args.includes('-s');

/**
 * @param {string} url
 * @param {number} retryCount
 * @param {number} maxRetries
 * @returns
 */
function connectToApp(url, retryCount = 0, maxRetries = 5) {
	return new Promise((resolve, reject) => {
		console.log(`[Attempt ${retryCount + 1}/${maxRetries + 1}] Connecting to ${url}...`);

		const ws = new WebSocket(url);
		const startTime = Date.now();
		let messageCount = 0;
		let waitingForSlowResponse = false;
		let pingInterval;
		let connectionEstablished = false;

		const cleanup = () => {
			if (pingInterval) {
				clearInterval(pingInterval);
				pingInterval = null;
			}
		};

		ws.on('open', () => {
			connectionEstablished = true;
			console.log(`Connected to ${url}`);

			// Send an initial message - either slow or regular
			if (useSlowEndpoint) {
				console.log('Sending slow request via WebSocket...');
				waitingForSlowResponse = true;
				ws.send('SLOW_REQUEST');
			} else {
				ws.send('Hello from Node.js client!');

				// Send periodic messages to keep the connection alive (regular mode only)
				pingInterval = setInterval(() => {
					if (ws.readyState === WebSocket.OPEN) {
						ws.send(`Ping from client at ${new Date().toISOString()}`);
					} else {
						cleanup();
					}
				}, 3000);
			}
		});

		ws.on('message', (data) => {
			messageCount++;
			const elapsed = (Date.now() - startTime) / 1000;
			const message = data.toString();
			console.log(`[${url}][${elapsed.toFixed(1)}s] ${message}`);

			// Handle slow mode responses
			if (useSlowEndpoint && waitingForSlowResponse) {
				if (message.startsWith('SLOW_COMPLETE') || message.startsWith('SLOW_INTERRUPTED')) {
					waitingForSlowResponse = false;
					console.log('Slow operation completed. Waiting 3 seconds before sending next slow request...');

					setTimeout(() => {
						if (ws.readyState === WebSocket.OPEN) {
							console.log('Sending another slow request via WebSocket...');
							waitingForSlowResponse = true;
							ws.send(`SLOW_PING at ${new Date().toISOString()}`);
						}
					}, 3000);
				}
			}
		});

		ws.on('close', (code, reason) => {
			cleanup();
			const totalTime = (Date.now() - startTime) / 1000;
			console.log(`[${url}] Connection closed after ${totalTime.toFixed(1)}s`);
			console.log(`[${url}] Received ${messageCount} messages`);
			console.log(`[${url}] Close code: ${code}, reason: ${reason.toString()}`);

			// Check if we should retry based on close code and retry count
			const shouldRetry = retryCount < maxRetries && (
				!connectionEstablished ||  // Initial connection failed
				code === 1006 ||          // Abnormal closure
				code === 1001 ||          // Going away (server restart)
				code === 1011 ||          // Server error
				code === 1012 ||          // Service restart
				code === 1013 ||          // Try again later
				code === 1014             // Bad gateway
			);

			if (shouldRetry) {
				// Retry connection
				const delay = Math.min(1000 * Math.pow(2, retryCount), 10000); // Exponential backoff, max 10s
				console.log(`Connection ${connectionEstablished ? 'dropped' : 'failed'}. Retrying in ${delay}ms...`);
				setTimeout(() => {
					connectToApp(url, retryCount + 1, maxRetries)
						.then(resolve)
						.catch(reject);
				}, delay);
			} else {
				// Don't retry - either max retries reached or clean close
				if (retryCount >= maxRetries) {
					reject(new Error(`Failed to maintain connection after ${maxRetries + 1} attempts`));
				} else {
					console.log(`Connection closed cleanly (code: ${code}). Not retrying.`);
					resolve();
				}
			}
		});

		ws.on('error', (error) => {
			cleanup();
			console.log(`[${url}] WebSocket error:`, error.message);

			// If we haven't established connection yet, let the close handler deal with retry
			if (!connectionEstablished) {
				// The close event will handle the retry logic
				return;
			}

			// If connection was established and then errored, try to reconnect
			const delay = Math.min(1000 * Math.pow(2, retryCount), 10000);
			console.log(`Connection error occurred. Retrying in ${delay}ms...`);
			setTimeout(() => {
				connectToApp(url, retryCount + 1, maxRetries)
					.then(resolve)
					.catch(reject);
			}, delay);
		});
	});
}

async function main() {
	const haproxyIngressNodePort = execSync(
		"kubectl get svc -n haproxy-controller \
		haproxy-ingress-kubernetes-ingress -o jsonpath='{.spec.ports[0].nodePort}'"
	)

	const ingressHost = execSync(
		"kubectl get ingress \
			-o jsonpath='{.items[0].spec.rules[0].host}'"
	)

	const url = `ws://${ingressHost}:${haproxyIngressNodePort}`;

	try {
		await connectToApp(url);
	} catch (error) {
		console.error('Error during WebSocket connection:', error);
		process.exit(1);
	}
}

// Handle graceful shutdown
process.on('SIGINT', () => {
	console.log('\nReceived SIGINT, shutting down gracefully...');
	process.exit(0);
});

process.on('SIGTERM', () => {
	console.log('\nReceived SIGTERM, shutting down gracefully...');
	process.exit(0);
});

main();