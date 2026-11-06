import { connect } from './trpc.mjs'

const MODULE = 'loupedeck'
const [version, ...pairs] = process.argv.slice(2)
if (version === 'group') {
	const { mutate, first, close } = await connect()
	try {
		const [{ info }] = await first('surfaces.watchSurfaces')
		const groups = Object.values(info)
		const physical = groups.flatMap(g => g.surfaces).find(s => s.integrationType === MODULE)
		const satellite = groups.flatMap(g => g.surfaces).find(s => s.id === 'zored-screen')
		if (!physical || !satellite) throw new Error('both Loupedeck and zored-screen surfaces must exist')
		const owner = groups.find(g => g.surfaces.some(s => s.id === physical.id))
		const sourceConfig = await first('surfaces.watchGroupConfig', { groupId: owner.id })
		if (!sourceConfig) throw new Error('physical surface group configuration is missing')
		const group = groups.find(g => g.id === 'group:zored-stream')
		const groupId = group?.id ?? await mutate('surfaces.groupAdd', { baseId: 'zored-stream', name: 'zored stream' })
		if (!group) {
			for (const [key, value] of Object.entries(sourceConfig)) {
				if (key !== 'name') await mutate('surfaces.groupSetConfigKey', { groupId, key, value })
			}
		}
		for (const surface of [physical, satellite]) {
			await mutate('surfaces.surfaceSetGroup', { surfaceId: surface.id, groupId })
		}
		console.log(`${physical.id} + ${satellite.id} → ${groupId}`)
	} finally { close() }
	process.exit(0)
}
if (!version) throw new Error('usage: node tools/surface.mjs <module version> [key=value ...]')
const config = Object.fromEntries(
	pairs.map((pair) => {
		const [key, value] = pair.split('=')
		if (!key || value === undefined) throw new Error(`expected key=value, got "${pair}"`)
		return [`plugin_cfg_${key}`, /^\d+$/.test(value) ? Number(value) : value]
	}),
)

const { mutate, first, close } = await connect()

async function moduleSurfaces() {
	const [{ info }] = await first('surfaces.watchSurfaces')
	return Object.values(info)
		.flatMap((group) => group.surfaces)
		.filter((surface) => surface.integrationType === MODULE && surface.isConnected)
}

async function configurable() {
	const keys = Object.keys(config)
	for (let i = 0; i < 60; i++) {
		const surfaces = await moduleSurfaces()
		const ready = surfaces.filter((s) => keys.every((key) => s.configFields.some((field) => field.id === key)))
		if (ready.length > 0) return ready
		await new Promise((resolve) => setTimeout(resolve, 500))
	}
	throw new Error(`no connected ${MODULE} surface with config fields ${keys.join(', ')}`)
}

try {
	const [{ info }] = await first('instances.surfaces.watch')
	const instance = Object.values(info).find((i) => i.moduleId === MODULE)
	if (!instance) throw new Error(`no ${MODULE} surface integration`)
	if (instance.moduleVersionId !== version) {
		const error = await mutate('instances.surfaces.setModuleAndVersion', {
			instanceId: instance.id,
			moduleId: MODULE,
			versionId: version,
		})
		if (error) throw new Error(`set ${MODULE} version ${version}: ${error}`)
	}
	console.log(`${MODULE} integration → ${version}`)

	for (const surface of await configurable()) {
		for (const [key, value] of Object.entries(config)) {
			await mutate('surfaces.surfaceSetConfigKey', { surfaceId: surface.id, key, value })
		}
		console.log(`${surface.id}: ${JSON.stringify(config)}`)
	}
} finally {
	close()
}
