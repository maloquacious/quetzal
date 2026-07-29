# Test Data

This folder holds three Zork story files: `zork1.z3`, `zork2.z3`, and `zork3.z3`.
Each one is a real Version 3 Z-machine story file.

They are here only as test fixtures. Tests that decode `CMem` need a real story
file, because `CMem` saves only the changes made to the story's original dynamic
memory.

Microsoft, Team Xbox, and Activision released the compiled Zork files under the
MIT License. Each story file was copied from that release. Each one has its own
license file, and that file is the one that applies. Read it before you reuse the
story file for anything.

| File       | Copied from | Source                                    | Upstream path       | License                                                     |
|------------|-------------|-------------------------------------------|---------------------|-------------------------------------------------------------|
| `zork1.z3` | Zork I      | https://github.com/historicalsource/zork1 | `COMPILED/zork1.z3` | MIT, Copyright (c) 2025 Microsoft — see `LICENSE.zork1.txt` |
| `zork2.z3` | Zork II     | https://github.com/historicalsource/zork2 | `COMPILED/zork2.z3` | MIT, Copyright (c) 2025 Microsoft — see `LICENSE.zork2.txt` |
| `zork3.z3` | Zork III    | https://github.com/historicalsource/zork3 | `COMPILED/zork3.z3` | MIT, Copyright (c) 2025 Microsoft — see `LICENSE.zork3.txt` |
