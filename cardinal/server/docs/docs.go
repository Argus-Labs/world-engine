// Package docs Code generated by swaggo/swag. DO NOT EDIT
package docs

import "github.com/swaggo/swag"

const docTemplate = `{
    "schemes": {{ marshal .Schemes }},
    "swagger": "2.0",
    "info": {
        "description": "{{escape .Description}}",
        "title": "{{.Title}}",
        "contact": {},
        "version": "{{.Version}}"
    },
    "host": "{{.Host}}",
    "basePath": "{{.BasePath}}",
    "paths": {
        "/cql": {
            "post": {
                "description": "Executes a CQL (Cardinal Query Language) query",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "summary": "Executes a CQL (Cardinal Query Language) query",
                "parameters": [
                    {
                        "description": "CQL query to be executed",
                        "name": "cql",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/handler.CQLQueryRequest"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "Results of the executed CQL query",
                        "schema": {
                            "$ref": "#/definitions/handler.CQLQueryResponse"
                        }
                    },
                    "400": {
                        "description": "Invalid request parameters",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        },
        "/debug/state": {
            "post": {
                "description": "Retrieves a list of all entities in the game state",
                "produces": [
                    "application/json"
                ],
                "summary": "Retrieves a list of all entities in the game state",
                "responses": {
                    "200": {
                        "description": "List of all entities",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/handler.debugStateElement"
                            }
                        }
                    }
                }
            }
        },
        "/events": {
            "get": {
                "description": "Establishes a new websocket connection to retrieve system events",
                "produces": [
                    "application/json"
                ],
                "summary": "Establishes a new websocket connection to retrieve system events",
                "responses": {
                    "101": {
                        "description": "Switch protocol to ws",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        },
        "/health": {
            "get": {
                "description": "Retrieves the status of the server and game loop",
                "produces": [
                    "application/json"
                ],
                "summary": "Retrieves the status of the server and game loop",
                "responses": {
                    "200": {
                        "description": "Server and game loop status",
                        "schema": {
                            "$ref": "#/definitions/handler.GetHealthResponse"
                        }
                    }
                }
            }
        },
        "/query/game/{queryName}": {
            "post": {
                "description": "Executes a query",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "summary": "Executes a query",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Name of a registered query",
                        "name": "queryName",
                        "in": "path",
                        "required": true
                    },
                    {
                        "description": "Query body as defined in its go type definition",
                        "name": "queryBody",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "type": "object"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "Results of the executed query",
                        "schema": {
                            "type": "object"
                        }
                    },
                    "400": {
                        "description": "Invalid request parameters",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        },
        "/query/receipts/list": {
            "post": {
                "description": "Retrieves all transaction receipts",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "summary": "Retrieves all transaction receipts",
                "parameters": [
                    {
                        "description": "Query body",
                        "name": "ListTxReceiptsRequest",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/cardinal.ListTxReceiptsRequest"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "List of receipts",
                        "schema": {
                            "$ref": "#/definitions/cardinal.ListTxReceiptsResponse"
                        }
                    },
                    "400": {
                        "description": "Invalid request body",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        },
        "/tx/game/{txName}": {
            "post": {
                "description": "Submits a transaction",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "summary": "Submits a transaction",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Name of a registered message",
                        "name": "txName",
                        "in": "path",
                        "required": true
                    },
                    {
                        "description": "Transaction details \u0026 message body as defined in its go type definition",
                        "name": "txBody",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/handler.Transaction"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "Transaction hash and tick",
                        "schema": {
                            "$ref": "#/definitions/handler.PostTransactionResponse"
                        }
                    },
                    "400": {
                        "description": "Invalid request parameter",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        },
        "/tx/persona/create-persona": {
            "post": {
                "description": "Creates a persona",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "summary": "Creates a persona",
                "parameters": [
                    {
                        "description": "Transaction details",
                        "name": "txBody",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/handler.Transaction"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "Transaction hash and tick",
                        "schema": {
                            "$ref": "#/definitions/handler.PostTransactionResponse"
                        }
                    },
                    "400": {
                        "description": "Invalid request parameter",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        },
        "/world": {
            "get": {
                "description": "Contains the registered components, messages, queries, and the Cardinal namespace",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "summary": "Retrieves details of the game world",
                "responses": {
                    "200": {
                        "description": "List of registered components, messages, queries, and the Cardinal namespace",
                        "schema": {
                            "$ref": "#/definitions/handler.GetWorldResponse"
                        }
                    },
                    "400": {
                        "description": "Invalid request parameters",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        }
    },
    "definitions": {
        "cardinal.ListTxReceiptsRequest": {
            "type": "object",
            "properties": {
                "startTick": {
                    "type": "integer"
                }
            }
        },
        "cardinal.ListTxReceiptsResponse": {
            "type": "object",
            "properties": {
                "endTick": {
                    "type": "integer"
                },
                "receipts": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/cardinal.ReceiptEntry"
                    }
                },
                "startTick": {
                    "type": "integer"
                }
            }
        },
        "cardinal.ReceiptEntry": {
            "type": "object",
            "properties": {
                "errors": {
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                },
                "result": {},
                "tick": {
                    "type": "integer"
                },
                "txHash": {
                    "type": "string"
                }
            }
        },
        "handler.CQLQueryRequest": {
            "type": "object",
            "properties": {
                "cql": {
                    "type": "string"
                }
            }
        },
        "handler.CQLQueryResponse": {
            "type": "object",
            "properties": {
                "results": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/handler.cqlData"
                    }
                }
            }
        },
        "handler.FieldDetail": {
            "type": "object",
            "properties": {
                "fields": {
                    "description": "variable name and type",
                    "type": "object",
                    "additionalProperties": {}
                },
                "name": {
                    "description": "name of the message or query",
                    "type": "string"
                },
                "url": {
                    "type": "string"
                }
            }
        },
        "handler.GetHealthResponse": {
            "type": "object",
            "properties": {
                "isGameLoopRunning": {
                    "type": "boolean"
                },
                "isServerRunning": {
                    "type": "boolean"
                }
            }
        },
        "handler.GetWorldResponse": {
            "type": "object",
            "properties": {
                "components": {
                    "description": "list of component names",
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/handler.FieldDetail"
                    }
                },
                "messages": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/handler.FieldDetail"
                    }
                },
                "namespace": {
                    "type": "string"
                },
                "queries": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/handler.FieldDetail"
                    }
                }
            }
        },
        "handler.PostTransactionResponse": {
            "type": "object",
            "properties": {
                "tick": {
                    "type": "integer"
                },
                "txHash": {
                    "type": "string"
                }
            }
        },
        "handler.Transaction": {
            "type": "object",
            "properties": {
                "body": {
                    "description": "json string",
                    "type": "object"
                },
                "hash": {
                    "type": "string"
                },
                "namespace": {
                    "type": "string"
                },
                "nonce": {
                    "type": "integer"
                },
                "personaTag": {
                    "type": "string"
                },
                "signature": {
                    "description": "hex encoded string",
                    "type": "string"
                }
            }
        },
        "handler.cqlData": {
            "type": "object",
            "properties": {
                "data": {
                    "type": "object"
                },
                "id": {
                    "type": "integer"
                }
            }
        },
        "handler.debugStateElement": {
            "type": "object",
            "properties": {
                "components": {
                    "type": "object"
                },
                "id": {
                    "type": "integer"
                }
            }
        }
    }
}`

// SwaggerInfo holds exported Swagger Info so clients can modify it
var SwaggerInfo = &swag.Spec{
	Version:          "0.0.1",
	Host:             "",
	BasePath:         "/",
	Schemes:          []string{"http", "ws"},
	Title:            "Cardinal",
	Description:      "Backend server for World Engine",
	InfoInstanceName: "swagger",
	SwaggerTemplate:  docTemplate,
	LeftDelim:        "{{",
	RightDelim:       "}}",
}

func init() {
	swag.Register(SwaggerInfo.InstanceName(), SwaggerInfo)
}
