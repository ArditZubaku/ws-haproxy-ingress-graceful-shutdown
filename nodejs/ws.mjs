import { execSync } from "node:child_process";
import WebSocket from 'ws';

// Parse command line arguments
const args = process.argv.slice(2);

// Show help if requested
if (args.includes('-h') || args.includes('--help')) {
	console.log('Usage: node ws.mjs [options]');
	console.log('Options:');
	console.log('  -s              Use slow endpoint mode');
	console.log('  -r              Enable automatic retries on connection failure');
	console.log('  -n <number>     Number of clients to spin up (default: 1)');
	console.log('  -h, --help      Show this help message');
	process.exit(0);
}

const useSlowEndpoint = args.includes('-s');
const enableRetries = args.includes('-r');

// Parse number of clients (-n <number>)
let numClients = 1;
const clientsIndex = args.indexOf('-n');
if (clientsIndex !== -1 && clientsIndex + 1 < args.length) {
	const clientsArg = parseInt(args[clientsIndex + 1], 10);
	if (!isNaN(clientsArg) && clientsArg > 0) {
		numClients = clientsArg;
	} else {
		console.error('Invalid number of clients. Using default value: 1');
	}
}

/**
 * @param {string} url
 * @param {number} retryCount
 * @param {number} maxRetries
 * @param {number} clientId
 * @returns
 */
function connectToApp(url, retryCount = 0, maxRetries = 5, clientId = 1) {
	return new Promise((resolve, reject) => {
		console.log(`[Client ${clientId}][Attempt ${retryCount + 1}/${maxRetries + 1}] Connecting to ${url}...`);

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
			console.log(`[Client ${clientId}] Connected to ${url}`);

			// Send an initial message - either slow or regular
			if (useSlowEndpoint) {
				console.log(`[Client ${clientId}] Sending slow request via WebSocket...`);
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
			console.log(`[Client ${clientId}][${url}][${elapsed.toFixed(1)}s] ${message}`);

			// Handle slow mode responses
			if (useSlowEndpoint && waitingForSlowResponse) {
				if (message.startsWith('SLOW_COMPLETE') || message.startsWith('SLOW_INTERRUPTED')) {
					waitingForSlowResponse = false;
					console.log(`[Client ${clientId}] Slow operation completed. Waiting 3 seconds before sending next slow request...`);

					setTimeout(() => {
						if (ws.readyState === WebSocket.OPEN) {
							console.log(`[Client ${clientId}] Sending another slow request via WebSocket...`);
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
			console.log(`[Client ${clientId}][${url}] Connection closed after ${totalTime.toFixed(1)}s`);
			console.log(`[Client ${clientId}][${url}] Received ${messageCount} messages`);
			console.log(`[Client ${clientId}][${url}] Close code: ${code}, reason: ${reason.toString()}`);

			// Only retry if -r flag is enabled
			if (!enableRetries) {
				console.log(`[Client ${clientId}] Connection closed. Retries disabled.`);
				resolve();
				return;
			}

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
				console.log(`[Client ${clientId}] Connection ${connectionEstablished ? 'dropped' : 'failed'}. Retrying in ${delay}ms...`);
				setTimeout(() => {
					connectToApp(url, retryCount + 1, maxRetries, clientId)
						.then(resolve)
						.catch(reject);
				}, delay);
			} else {
				// Don't retry - either max retries reached or clean close
				if (retryCount >= maxRetries) {
					reject(new Error(`Failed to maintain connection after ${maxRetries + 1} attempts`));
				} else {
					console.log(`[Client ${clientId}] Connection closed cleanly (code: ${code}). Not retrying.`);
					resolve();
				}
			}
		});

		ws.on('error', (error) => {
			cleanup();
			console.log(`[Client ${clientId}][${url}] WebSocket error:`, error.message);

			// If retries are disabled, don't attempt to reconnect
			if (!enableRetries) {
				console.log(`[Client ${clientId}] Connection error. Retries disabled.`);
				reject(error);
				return;
			}

			// If we haven't established connection yet, let the close handler deal with retry
			if (!connectionEstablished) {
				// The close event will handle the retry logic
				return;
			}

			// If connection was established and then errored, try to reconnect
			if (retryCount < maxRetries) {
				const delay = Math.min(1000 * Math.pow(2, retryCount), 10000);
				console.log(`[Client ${clientId}] Connection error occurred. Retrying in ${delay}ms...`);
				setTimeout(() => {
					connectToApp(url, retryCount + 1, maxRetries, clientId)
						.then(resolve)
						.catch(reject);
				}, delay);
			} else {
				reject(new Error(`Failed to connect after ${maxRetries + 1} attempts: ${error.message}`));
			}
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

	console.log(`Starting ${numClients} WebSocket client${numClients > 1 ? 's' : ''}...`);

	try {
		// Create array of connection promises for multiple clients
		const connectionPromises = [];
		for (let i = 1; i <= numClients; i++) {
			connectionPromises.push(connectToApp(url, 0, 5, i));
		}

		// Wait for all clients to complete
		await Promise.allSettled(connectionPromises);
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