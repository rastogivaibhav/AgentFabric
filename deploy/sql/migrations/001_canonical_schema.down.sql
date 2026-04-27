DROP INDEX IF EXISTS idx_tool_calls_user;
DROP INDEX IF EXISTS idx_tool_calls_risk;
DROP INDEX IF EXISTS idx_tool_calls_tool;
DROP INDEX IF EXISTS idx_tool_calls_time;
DROP TABLE IF EXISTS ai_agent_tool_calls;

DROP INDEX IF EXISTS idx_usage_user;
DROP INDEX IF EXISTS idx_usage_source;
DROP INDEX IF EXISTS idx_usage_time;
DROP TABLE IF EXISTS ai_agent_usage;

DROP INDEX IF EXISTS idx_ai_events_risk;
DROP INDEX IF EXISTS idx_ai_events_session;
DROP INDEX IF EXISTS idx_ai_events_user;
DROP INDEX IF EXISTS idx_ai_events_source;
DROP INDEX IF EXISTS idx_ai_events_time;
DROP TABLE IF EXISTS ai_agent_events;
