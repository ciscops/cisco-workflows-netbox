# Overview

This script is designed to generate AO Atomics based on [Meraki OpenAPI specifications](https://raw.githubusercontent.com/meraki/openapi/refs/heads/master/openapi/spec3.json).

# DISCLAIMER

This script is for internal usage only and requires testing the atomic manually after generation. 
it has only been tested with Meraki openAPI spec so far and will require further development to support other external endpoints.

## Features

- Parses OpenAPI JSON files to extract operations.
- Generates workflows with input and output variables.
- Supports idempotency with customizable conditions.
- Allows categorization of workflows.
- Generates path and query parameters as user inputs:
  - Path params are required and hidden from the wizard ("Input - <Name>").
  - Query params are visible in the wizard and prefixed with "Query - <Name>"; required flags follow the OpenAPI spec.

## Prerequisites

- Go programming language installed on your system.
- [Meraki OpenAPI spec](https://raw.githubusercontent.com/meraki/openapi/refs/heads/master/openapi/spec3.json) downloaded locally.

## Usage

update permission on the executable
```bash
chmod +x generate_workflow
```
To generate a workflow, use the `go run` command with the necessary flags:

```bash
./generate_workflow \
  -openapi=/Users/oelmouat/workspace/ao/ao-atomic-generator/meraki_specs_v3.json \
  -operationId=createOrganizationNetwork \
  -platform=Meraki \
  -supportIdempotency=true \
  -idempotencyCondition="Name has already been taken" \
  -categoryId=category_02LJAJ25TKWKJ2JWbrsG5z2UzO6wBxu5BLi \
  -categoryName="Cisco Meraki - Wireless"
  
  -openapi string
    	Path to the OpenAPI JSON file.
  -operationId string
    	The operationId to use from the OpenAPI spec.
  -supportIdempotency
    	whether the atomic should support idempotency. default to false
  -idempotencyCondition string
    	Error Message to use decide if idempotency is enabled. for POST use the error message and for DELETE/GET/PUT input the error code (most of the time 404)
-categoryId string
    	the Category Id to put the atomic under.
  -categoryName string
    	the Category Name to put the atomic under.
  -platform string
    	Optional platform prefix for names and titles (e.g., 'Meraki').
```