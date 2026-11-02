const url = process.env.COMPANION_TRPC ?? 'ws://127.0.0.1:8000/trpc'

export async function connect() {
	const ws = new WebSocket(url)
	const pending = new Map()
	let nextId = 1

	ws.addEventListener('message', (event) => {
		const msg = JSON.parse(event.data)
		const call = pending.get(msg.id)
		if (!call) return
		if (msg.error) {
			pending.delete(msg.id)
			call.reject(new Error(`${call.path}: ${msg.error.message}`))
		} else if (call.method === 'mutation') {
			pending.delete(msg.id)
			call.resolve(msg.result?.data)
		} else if (msg.result?.type === 'data') {
			pending.delete(msg.id)
			ws.send(JSON.stringify({ id: msg.id, method: 'subscription.stop' }))
			call.resolve(msg.result.data)
		}
	})
	ws.addEventListener('close', () => {
		for (const call of pending.values()) call.reject(new Error(`${call.path}: connection closed`))
	})

	function send(method, path, input) {
		const id = nextId++
		return new Promise((resolve, reject) => {
			pending.set(id, { method, path, resolve, reject })
			ws.send(JSON.stringify({ id, method, params: { path, input } }))
		})
	}

	await new Promise((resolve, reject) => {
		ws.addEventListener('open', resolve, { once: true })
		ws.addEventListener('error', () => reject(new Error(`can't connect to ${url}`)), { once: true })
	})

	return {
		mutate: (path, input) => send('mutation', path, input),
		first: (path, input) => send('subscription', path, input),
		close: () => ws.close(),
	}
}
