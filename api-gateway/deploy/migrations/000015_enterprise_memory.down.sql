DROP INDEX IF EXISTS idx_evidence_bundle_items_tenant_type_created;
DROP INDEX IF EXISTS idx_evidence_bundle_items_bundle_created;
DROP TABLE IF EXISTS evidence_bundle_items;

DROP INDEX IF EXISTS idx_evidence_bundles_tenant_scope_created;
DROP INDEX IF EXISTS idx_evidence_bundles_tenant_created;
DROP TABLE IF EXISTS evidence_bundles;

DROP INDEX IF EXISTS idx_control_history_tenant_target_created;
DROP INDEX IF EXISTS idx_control_history_tenant_category_created;
DROP INDEX IF EXISTS idx_control_history_tenant_created;
DROP TABLE IF EXISTS control_history;
