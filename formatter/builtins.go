package formatter

import "strings"

// builtinFunctions lists common PostgreSQL built-in functions (lower case). It is
// used by builtin_functions.case; names not listed here are treated as user
// functions. Functions with dedicated syntax (COALESCE, GREATEST, ...) are
// handled by conditional_functions and are not repeated here.
var builtinFunctions = func() map[string]bool {
	m := map[string]bool{}
	for _, n := range strings.Fields(`
		count sum avg min max array_agg string_agg json_agg jsonb_agg json_object_agg jsonb_object_agg
		bool_and bool_or every bit_and bit_or stddev stddev_pop stddev_samp variance var_pop var_samp
		corr covar_pop covar_samp percentile_cont percentile_disc mode xmlagg
		row_number rank dense_rank percent_rank cume_dist ntile lag lead first_value last_value nth_value
		abs ceil ceiling floor round trunc mod power sqrt cbrt exp ln log log10 sign random setseed
		div gcd lcm degrees radians pi sin cos tan asin acos atan atan2 width_bucket
		lower upper initcap length char_length character_length octet_length bit_length
		substr substring left right trim btrim ltrim rtrim lpad rpad replace translate repeat reverse
		concat concat_ws format position strpos split_part starts_with regexp_replace regexp_match
		regexp_matches regexp_split_to_array regexp_split_to_table md5 encode decode to_hex ascii chr
		quote_ident quote_literal quote_nullable overlay
		now clock_timestamp statement_timestamp transaction_timestamp timeofday age date_trunc date_part
		extract date_bin make_date make_time make_timestamp make_timestamptz make_interval
		to_char to_date to_number to_timestamp justify_days justify_hours justify_interval isfinite
		array_length array_append array_prepend array_cat array_remove array_replace array_position
		array_positions array_dims array_lower array_upper cardinality unnest generate_series
		generate_subscripts string_to_array array_to_string
		json_build_object jsonb_build_object json_build_array jsonb_build_array json_array_elements
		jsonb_array_elements json_array_elements_text jsonb_array_elements_text json_each jsonb_each
		json_each_text jsonb_each_text json_extract_path jsonb_extract_path json_extract_path_text
		jsonb_extract_path_text json_object_keys jsonb_object_keys json_typeof jsonb_typeof
		jsonb_set jsonb_insert jsonb_pretty jsonb_strip_nulls json_strip_nulls to_json to_jsonb
		row_to_json jsonb_path_query jsonb_path_exists jsonb_array_length json_array_length
		to_tsvector to_tsquery plainto_tsquery phraseto_tsquery websearch_to_tsquery ts_rank ts_rank_cd ts_headline
		gen_random_uuid uuid_generate_v4 pg_typeof pg_sleep pg_size_pretty pg_total_relation_size
		pg_relation_size pg_table_size pg_database_size pg_get_viewdef pg_get_functiondef pg_get_expr
		current_setting set_config txid_current inet_client_addr inet_server_addr version
		nextval currval setval lastval has_table_privilege has_schema_privilege
		isempty lower_inc upper_inc lower_inf upper_inf
	`) {
		m[n] = true
	}
	return m
}()

func isBuiltinFunction(name string) bool {
	return builtinFunctions[strings.ToLower(name)]
}
