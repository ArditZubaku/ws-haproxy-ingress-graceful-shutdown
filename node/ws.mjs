import WebSocket from 'ws';

// Parse command line arguments
const args = process.argv.slice(2);
const useSlowEndpoint = args.includes('-s');

/**
 * @param {WebSocket} ws
 * @param {string} url
 * @returns
 */
function connectToApp(ws, url) {
	return new Promise((_resolve, reject) => {
		console.log(`Connecting to ${url}...`);

		const startTime = Date.now();
		let messageCount = 0;

		ws.on('open', () => {
			console.log(`Connected to ${url}`);

			// Send an initial message - either slow or regular
			if (useSlowEndpoint) {
				console.log('Sending slow request via WebSocket...');
				ws.send('SLOW_REQUEST');
			} else {
				ws.send('Hello from Node.js client!');
			}

			// Send periodic messages to keep the connection alive
			const interval = setInterval(() => {
				if (ws.readyState === WebSocket.OPEN) {
					ws.send(`Ping from client at ${new Date().toISOString()}`);
				} else {
					clearInterval(interval);
				}
			}, 3000); // Send a message every 3 seconds

			// Clean up interval when connection closes
			ws.on('close', () => {
				clearInterval(interval);
			});
		});

		ws.on('message', (data) => {
			messageCount++;
			const elapsed = (Date.now() - startTime) / 1000;
			console.log(`[${url}][${elapsed.toFixed(1)}s] ${data.toString()}`);
		});

		ws.on('close', (code, reason) => {
			const totalTime = (Date.now() - startTime) / 1000;
			console.log(`[${url}] Connection closed after ${totalTime.toFixed(1)}s`);
			console.log(`[${url}] Received ${messageCount} messages`);
			console.log(`[${url}] Close code: ${code}, reason: ${reason.toString()}`);
		});

		ws.on('error', (error) => {
			console.log(`[${url}] WebSocket error:`, error.message);
			reject(error);
		});
	});
}

async function main() {
	const url = `ws://127.0.0.1:8080`;
	const ws = new WebSocket(url);

	try {
		await connectToApp(ws, url);
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