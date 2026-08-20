# PI4.3 execution marker

This branch was used only to trigger the isolated, label-free PI4.3 fixed-model experiment and must not be merged into AgentFabric.

Frozen prompt SHA-256: `833cac63e7c55f5fe227d6b269dd517e37fb664b0c6d7223702751657a003eb9`

Executor model: `Qwen/Qwen2.5-1.5B-Instruct-GGUF`, Q4_K_M, SHA-256 `6a1a2eb6d15622bf3c96857206351ba97e1af16c30d7a74ee38970e434e9407e`.

Gold labels were not present or available to the executor.

Execution completed successfully on run `32379059074`; predictions SHA-256: `e3e3e2f2d35126716194137f40cf199a1afa527731eee4445d32b014d20439eb`.

Frozen evaluator result: **FAIL — promotion gate not satisfied**, despite a material utility gain: RAW 48.33% -> G5C 65.83% (+17.50 pp; paired bootstrap 95% CI +9.17 to +25.83 pp). Failure reasons: contested recall remained 0%, and unsupported-promotion rate worsened from 45.0% RAW to 60.0% G5C.

Scientific interpretation: stronger-model GrapheneDB evidence utility is supported, but epistemic safety/promotion is not. This branch remains execution-only and must not be merged.
