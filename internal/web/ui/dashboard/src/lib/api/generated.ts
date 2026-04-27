// AUTO-GENERATED — DO NOT EDIT.
//
// `make types` runs `tygo generate` (config: tools/tygo/config.yaml)
// and overwrites this file. CI fails on uncommitted drift.
//
// Until that pipeline first runs locally, this stub keeps the import
// path stable so `useApiQuery<KnobDef>(...)` infers correctly when the
// upstream Go struct lives in `internal/settings/catalog.go`. The
// fields below are kept in lockstep with the canonical Go shape; any
// new field must also be added to internal/settings/catalog.go's
// KnobDef and re-exported via `make types`.
export interface KnobDef {
  key: string;
  type: 'int' | 'float' | 'string' | 'enum' | 'multiselect';
  slider_min?: number;
  slider_max?: number;
  slider_step?: number;
  enum_values?: string[];
  default: unknown;
  description: string;
  unit?: string;
}
