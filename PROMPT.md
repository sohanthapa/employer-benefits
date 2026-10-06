# Prompt

The prompt used to build this project with an AI coding assistant.

---

Only Plan for now - no coding.

We would like to build an AI agent for employer benefits called "Employer Benefits Agent". This work will only be for backend service using Golang. We do not need to worry about authorization and authentication.

The agent has access to these tools:

- `get_plan_details(employee_id)` — returns the employee's benefit coverage
- `search_providers(zip_code, specialty)` — finds matching providers
- `create_support_ticket(reason)` — escalates unresolved questions

The agent should decide which tools to call, collect any missing information, and provide an accurate response.

Example request from the user: "Does my plan cover family counseling, and can you help me find a provider near me?"

## Mock data

For the simplicity of this exercise we can mock the plan details, location (zip code) and specialty using a json file stored under docs folder.

The types of specialty supported are:

- `family_counseling`
- `marriage_counseling`
- `anxiety`
- `depression`
- `substance_use`
- `child_therapy`

## Rules for the agent

1. Only use the available tools given above, do not invent new tools.
2. You may call the tools maximum of 5 times per user request. If you reach 5, stop and create a support ticket. The `create_support_ticket` call does not count toward the 5, so we can always escalate.
3. Only search the specialty the user asked for. If no provider is found, create a support ticket. Do not replace it with another specialty.
4. If employee_id, zip code or specialty is missing from the request, ask the user for it. Do not guess.
5. Only answer coverage questions using the output from `get_plan_details`. Do not answer from your own knowledge and do not give medical advice.
6. Create a support ticket when:
   - the tool returns an error
   - the employee is not found
   - no provider is found
   - the specialty asked is not in the supported list
   - the 5 tool call limit is reached

   The ticket reason should say what the user asked and why we could not resolve it.

## Response

Each response should return the answer, the list of tools called, and the ticket id if a support ticket was created. This way I can check what the agent did.

## How to write the code

- Write it in small slices such that each slice of code can be compiled, run and tested successfully. This way we are working in small chunks and I can run the code and see the output after each slice.
- Each slice should have unit tests.
- Add code comments, but make it concise and precise, avoid any optional details/description in the code. The comments should be like how an engineer would write and easy to read and follow.

## Review

Spawn a reviewer agent and after completing each slice of work have the reviewer agent review the code, then address the feedback. You can avoid nit comments. Log those comments and feedback in a new file (just for reference purpose).

## Scenarios the final code should pass

1. Covered and providers found — answer with coverage and providers
2. Not covered — say it is not covered, no provider search
3. Covered but no provider found — create a support ticket
4. Missing zip code — ask the user for it
5. Employee not found — create a support ticket
6. Specialty not supported — create a support ticket
7. 5 tool call limit reached — create a support ticket

If something is unclear ask me during the planning phase and do not make any assumption.
