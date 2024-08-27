const fs = require('fs')

/// ---------------------------------------------------------------------------
/// Initial setup
/// ---------------------------------------------------------------------------

const openapiPath = process.argv[2]
const file = fs.readFileSync(openapiPath)
const data = JSON.parse(file)


/// ---------------------------------------------------------------------------
/// Rename SDK methods/functions
/// ---------------------------------------------------------------------------

data.paths['/cql'].post['x-speakeasy-name-override'] = 'queryCql'
data.paths['/debug/state'].post['x-speakeasy-name-override'] = 'getDebugState'
data.paths['/health'].get['x-speakeasy-name-override'] = 'getHealth'
data.paths['/query/game/{queryName}'].post['x-speakeasy-name-override'] = 'query'
data.paths['/query/receipts/list'].post['x-speakeasy-name-override'] = 'getReceipts'
data.paths['/tx/game/{txName}'].post['x-speakeasy-name-override'] = 'transact'
data.paths['/tx/persona/create-persona'].post['x-speakeasy-name-override'] = 'createPersona'
data.paths['/world'].get['x-speakeasy-name-override'] = 'getWorld'


/// ---------------------------------------------------------------------------
/// Hide endpoints from SDK generation
/// ---------------------------------------------------------------------------

// Speakeasy doesn't do websockets
data.paths['/events'].get['x-speakeasy-ignore'] = true
// These are for use in cardinal internals
data.paths['/query/{queryGroup}/{queryName}'].post['x-speakeasy-ignore'] = true
data.paths['/tx/{txGroup}/{txName}'].post['x-speakeasy-ignore'] = true


/// ---------------------------------------------------------------------------
/// SDK global parameters
///
/// Speakeasy doesn't support additional SDK init options that can be used in hooks,
/// so the workaround here is to set these as query parameters that can be set
/// globally when initializing the SDK. The custom hooks can then get these
/// options from the request url. A downside of this is that only primitive types
/// are supported as global params.
/// ---------------------------------------------------------------------------

// Private key used to sign transactions (messages)
const privateKeyParam = {
  name: '_privateKey',
  in: 'query',
  schema: {
    type: 'string'
  },
}
// Cardinal namespace
const namespaceParam = {
  name: '_namespace',
  in: 'query',
  schema: {
    type: 'string'
  },
}

// Use x-speakeasy-globals-hidden to hide the params from method/function signatures
data['x-speakeasy-globals'] = {
  parameters: [
    {
      ...privateKeyParam,
      'x-speakeasy-globals-hidden': true
    },
    {
      ...namespaceParam,
      'x-speakeasy-globals-hidden': true
    }
  ]
}

data.paths['/tx/game/{txName}'].post.parameters.push(privateKeyParam)
data.paths['/tx/game/{txName}'].post.parameters.push(namespaceParam)

// create-persona has no parameters so we need to set the initial empty array
data.paths['/tx/persona/create-persona'].post.parameters = []
data.paths['/tx/persona/create-persona'].post.parameters.push(privateKeyParam)
data.paths['/tx/persona/create-persona'].post.parameters.push(namespaceParam)


/// ---------------------------------------------------------------------------
/// Fix object types in SDK generation
///
/// Swagger doesn't support `additionalProperties` for object types. This is needed
/// for open maps, i.e. objects/hashmaps/dictionaries with dynamic keys. We must
/// set this after the conversion to OpenAPI 3 by swagger-codegen. The reason for this
/// is that without the `additionalProperties` key, Speakeasy will set the type of
/// the schema to `type Typename = {}`, which when coerced by zod will always result
/// in an empty object. With `additionalProperties`, the type generated will be
/// `type Typename = { [k: string]: any }`, which is what we want.
/// ---------------------------------------------------------------------------

// POST /cql
delete data.components.schemas['pkg_world_dev_world-engine_cardinal_types.EntityStateElement'].properties.data.items.properties
data.components.schemas['pkg_world_dev_world-engine_cardinal_types.EntityStateElement'].properties.data.items.additionalProperties = {}

// POST /debug/state
delete data.components.schemas['pkg_world_dev_world-engine_cardinal_types.DebugStateElement'].properties.components.properties
data.components.schemas['pkg_world_dev_world-engine_cardinal_types.DebugStateElement'].properties.components.additionalProperties = {}

// POST /query/game/{queryName}
data.paths['/query/game/{queryName}'].post.requestBody.content['application/json'].schema.additionalProperties = {}
data.paths['/query/game/{queryName}'].post.responses['200'].content['application/json'].schema.additionalProperties = {}

// POST /tx/persona/create-persona, POST /tx/game/{txName}
delete data.components.schemas['cardinal_server_handler.Transaction'].properties.body.properties
data.components.schemas['cardinal_server_handler.Transaction'].properties.body.additionalProperties = {}

// GET /world
data.components.schemas['pkg_world_dev_world-engine_cardinal_types.FieldDetail'].properties.fields.additionalProperties = {}


/// ---------------------------------------------------------------------------
/// Rename types
/// ---------------------------------------------------------------------------

// Cardinal entity types
data.components.schemas['pkg_world_dev_world-engine_cardinal_types.EntityStateElement']['x-speakeasy-name-override'] = 'EntityStateElement'

// POST /cql
data.components.schemas['cardinal_server_handler.CQLQueryRequest']['x-speakeasy-name-override'] = 'CQLQueryRequest'
data.components.schemas['cardinal_server_handler.CQLQueryResponse']['x-speakeasy-name-override'] = 'CQLQueryResponse'

// POST /tx/persona/create-persona, POST /tx/game/{txName}
data.components.schemas['cardinal_server_handler.Transaction']['x-speakeasy-name-override'] = 'txBody'

// data.paths['/query/game/{queryName}'].post.requestBody['x-speakeasy-name-override'] = 'queryBody'


/// ---------------------------------------------------------------------------
/// Apply changes
/// ---------------------------------------------------------------------------

try {
  fs.writeFileSync(openapiPath, JSON.stringify(data, null, 2))
  console.log('Updated openapi.json with speakeasy attributes')
} catch (error) {
  console.log('Error updating openapi.json:', error)
}
