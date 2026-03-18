"""
Token usage & cost CSV analysis app (analyst-oriented layout)

Analysis flow:
  1. Summary: data scale (KPIs) → summary stats (single table) → segments (by model) → raw preview
  2. Charts: distributions → model comparison → box plots (outliers & spread)
  3. Clustering/efficiency: visualize efficiency clusters by request/model/cache ratio
"""
import math
import os
from typing import Optional

import numpy as np
import pandas as pd
import plotly.express as px
import streamlit as st

try:
    from sklearn.cluster import KMeans  # type: ignore[import]
except Exception:  # Graceful degradation if sklearn is unavailable
    KMeans = None

COL_TOKENS = "Total Tokens"
COL_COST = "Cost"
COL_MODEL = "Model"
COL_DATE = "Date"
COL_CACHE_READ = "Cache Read"


def safe_div(a, b):
    """Scalar division: return None if denominator is 0/None/NaN."""
    if b is None:
        return None
    try:
        if float(b) == 0.0:
            return None
    except Exception:
        return None
    try:
        return a / b
    except Exception:
        return None


def safe_div_series(num: pd.Series, den: pd.Series) -> pd.Series:
    """Series division: return NaN where denominator is 0/NaN."""
    den = den.copy()
    den = den.replace(0, pd.NA)
    return num / den


def fmt_val(x, fmt: str, na: str = "NA") -> str:
    """Formatter that renders NaN/None/inf as NA."""
    try:
        if x is None:
            return na
        if isinstance(x, (float, int)):
            if math.isnan(float(x)) or math.isinf(float(x)):
                return na
        return fmt.format(x)
    except Exception:
        return na


def load_csv(path: str) -> pd.DataFrame | None:
    try:
        df = pd.read_csv(path)
        if COL_TOKENS not in df.columns or COL_COST not in df.columns:
            st.error(f"Missing required columns: '{COL_TOKENS}', '{COL_COST}'")
            return None
        if df[COL_COST].dtype == object:
            df[COL_COST] = df[COL_COST].astype(str).str.replace("$", "").str.strip()
        df[COL_COST] = pd.to_numeric(df[COL_COST], errors="coerce")
        df[COL_TOKENS] = pd.to_numeric(df[COL_TOKENS], errors="coerce")
        # For datasets without real numeric Cost values (e.g., usage-events where Cost can be 'Included'),
        # allow loading as long as tokens are present; keep Cost as NaN.
        df = df.dropna(subset=[COL_TOKENS])
        if COL_MODEL in df.columns:
            df[COL_MODEL] = df[COL_MODEL].astype(str)
        return df
    except Exception as e:
        st.error(f"Failed to load CSV: {e}")
        return None


def get_csv_list():
    """Return sorted list of CSV files in the analyze folder."""
    analyze_dir = os.path.dirname(os.path.abspath(__file__))
    files = [f for f in os.listdir(analyze_dir) if f.endswith(".csv")]
    return sorted(files)


def ensure_data():
    csv_files = get_csv_list()
    if not csv_files:
        return None, None, []
    selected = st.session_state.get("selected_csv", csv_files[0])
    if selected not in csv_files:
        selected = csv_files[0]
    analyze_dir = os.path.dirname(os.path.abspath(__file__))
    path = os.path.join(analyze_dir, selected)
    df = load_csv(path)
    return df, path, csv_files


def _parse_date_range(df: pd.DataFrame):
    """Date range info for timeseries context."""
    if COL_DATE not in df.columns:
        return None, None
    s = pd.to_datetime(df[COL_DATE], errors="coerce").dropna()
    if s.empty:
        return None, None
    return s.min(), s.max()


def render_summary(df: pd.DataFrame):
    st.header("Summary")

    total_tokens = df[COL_TOKENS].sum()
    total_cost = df[COL_COST].sum()

    # Context: understand the time span and volume first.
    n_rows = len(df)
    date_min, date_max = _parse_date_range(df)
    if date_min is not None and date_max is not None:
        st.caption(
            f"📌 **Data**: {n_rows:,} requests · {date_min.strftime('%Y-%m-%d')} ~ {date_max.strftime('%Y-%m-%d')}"
        )
    else:
        st.caption(f"📌 **Data**: {n_rows:,} requests")

    avg_cost_per_token = safe_div(total_cost, total_tokens)
    avg_tokens_per_dollar = safe_div(total_tokens, total_cost)

    # 1) Scale first: totals + average unit cost.
    st.subheader("1. Overall scale (KPIs)")
    st.caption("Goal: quickly understand total cost/tokens and average unit cost.")
    col1, col2, col3 = st.columns(3)
    with col1:
        st.metric("Total tokens", f"{total_tokens:,.0f}")
    with col2:
        st.metric(
            "Avg cost per token",
            "NA" if avg_cost_per_token is None else f"${avg_cost_per_token:.6f}",
        )
    with col3:
        st.metric(
            "Avg tokens per $1",
            "NA" if avg_tokens_per_dollar is None else f"{avg_tokens_per_dollar:,.0f}",
        )

    df = df.copy()
    df["_cost_per_token"] = safe_div_series(df[COL_COST], df[COL_TOKENS])
    df["_tokens_per_dollar"] = safe_div_series(df[COL_TOKENS], df[COL_COST])

    # 2) One summary table: compare min/max/mean/range side-by-side.
    st.subheader("2. Per-request summary stats (min / max / mean / range)")
    st.caption("Goal: compare spread and potential outliers across metrics in one view.")
    summary = pd.DataFrame({
        "Metric": ["Cost ($)", "Total Tokens", "Cost per token ($/token)", "Tokens per $ (per request)"],
        "Min": [
            fmt_val(df[COL_COST].min(), "${:.4f}"),
            fmt_val(df[COL_TOKENS].min(), "{:,.0f}"),
            fmt_val(df["_cost_per_token"].min(), "${:.6f}"),
            fmt_val(df["_tokens_per_dollar"].min(), "{:,.0f}"),
        ],
        "Max": [
            fmt_val(df[COL_COST].max(), "${:.4f}"),
            fmt_val(df[COL_TOKENS].max(), "{:,.0f}"),
            fmt_val(df["_cost_per_token"].max(), "${:.6f}"),
            fmt_val(df["_tokens_per_dollar"].max(), "{:,.0f}"),
        ],
        "Mean": [
            fmt_val(df[COL_COST].mean(), "${:.4f}"),
            fmt_val(df[COL_TOKENS].mean(), "{:,.0f}"),
            fmt_val(df["_cost_per_token"].mean(), "${:.6f}"),
            fmt_val(df["_tokens_per_dollar"].mean(), "{:,.0f}"),
        ],
        "Range": [
            fmt_val(df[COL_COST].max() - df[COL_COST].min(), "${:.4f}"),
            fmt_val(df[COL_TOKENS].max() - df[COL_TOKENS].min(), "{:,.0f}"),
            fmt_val(df["_cost_per_token"].max() - df["_cost_per_token"].min(), "${:.6f}"),
            fmt_val(df["_tokens_per_dollar"].max() - df["_tokens_per_dollar"].min(), "{:,.0f}"),
        ],
    })
    st.dataframe(summary, use_container_width=True, hide_index=True)

    st.divider()
    # 3) Segment: compare cost share and unit economics by model.
    st.subheader("3. Cost & tokens by model (segment)")
    st.caption("Goal: compare cost share and unit costs by model to find optimization opportunities.")

    if COL_MODEL not in df.columns:
        st.info("No 'Model' column found, skipping model-level aggregation.")
    elif df[COL_COST].dropna().empty or df[COL_COST].fillna(0).sum() <= 0:
        st.info("No Cost values found; skipping model-level cost/unit-cost stats. You can still review token usage in the raw preview below.")
    else:
        by_model = df.groupby(COL_MODEL, as_index=False).agg(
            Requests=(COL_COST, "count"),
            Total_Tokens=(COL_TOKENS, "sum"),
            Total_Cost=(COL_COST, "sum"),
        )
        by_model["Avg_Cost_Per_Token"] = safe_div_series(by_model["Total_Cost"], by_model["Total_Tokens"])
        by_model["Tokens_Per_$"] = safe_div_series(by_model["Total_Tokens"], by_model["Total_Cost"])
        by_model["Total_Cost"] = by_model["Total_Cost"].round(4)
        by_model["Avg_Cost_Per_Token"] = by_model["Avg_Cost_Per_Token"].round(6)
        by_model["Tokens_Per_$"] = by_model["Tokens_Per_$"].round(0)

        st.dataframe(
            by_model.style.format({
                "Total_Tokens": "{:,.0f}",
                "Total_Cost": "${:.4f}",
                "Avg_Cost_Per_Token": "${:.6f}",
                "Tokens_Per_$": "{:,.0f}",
            }),
            use_container_width=True,
            hide_index=True,
        )

    st.divider()
    st.subheader("4. Raw data preview")
    st.caption("Goal: inspect outliers/missing values and explore details.")
    st.write(f"Total cost **${total_cost:.2f}** · Total tokens **{total_tokens:,.0f}** · Rows **{len(df):,}**")
    with st.expander("Show top 100 rows"):
        cols = [c for c in [COL_MODEL, COL_TOKENS, COL_COST] if c in df.columns]
        st.dataframe(df[cols].head(100) if cols else df.head(100), use_container_width=True)


def render_graphs(df: pd.DataFrame):
    st.header("Charts")

    if df is None or df.empty:
        st.warning("No data to display. Please select a CSV on the Summary page.")
        return

    st.caption("**Flow**: distributions → model comparison → box plots (outliers & spread).")

    df = df.copy()
    df["_cost_per_token"] = safe_div_series(df[COL_COST], df[COL_TOKENS])
    df["_tokens_per_dollar"] = safe_div_series(df[COL_TOKENS], df[COL_COST])

    tab1, tab2, tab3 = st.tabs(["Distributions", "By model", "Box plots"])

    with tab1:
        st.subheader("Distributions (histograms)")
        st.caption("Goal: inspect the overall distribution, skew, and potential outliers for cost/tokens/unit metrics.")
        c1, c2 = st.columns(2)
        with c1:
            fig = px.histogram(df, x=COL_COST, nbins=50, title="Cost distribution")
            fig.update_layout(showlegend=False)
            st.plotly_chart(fig, use_container_width=True)
        with c2:
            fig = px.histogram(df, x=COL_TOKENS, nbins=50, title="Total tokens distribution")
            fig.update_layout(showlegend=False)
            st.plotly_chart(fig, use_container_width=True)
        c1, c2 = st.columns(2)
        with c1:
            fig = px.histogram(df, x="_cost_per_token", nbins=50, title="Cost per token ($/token) distribution")
            fig.update_layout(showlegend=False)
            st.plotly_chart(fig, use_container_width=True)
        with c2:
            fig = px.histogram(df, x="_tokens_per_dollar", nbins=50, title="Tokens per $ (per request) distribution")
            fig.update_layout(showlegend=False)
            st.plotly_chart(fig, use_container_width=True)

    with tab2:
        st.subheader("By model (bars)")
        st.caption("Goal: compare total cost, average unit cost, and tokens per $ across models.")
        if COL_MODEL not in df.columns:
            st.info("No 'Model' column found, skipping model-level charts.")
        else:
            by_model = df.groupby(COL_MODEL).agg(
                Total_Cost=(COL_COST, "sum"),
                Total_Tokens=(COL_TOKENS, "sum"),
            ).reset_index()
            by_model["Avg_Cost_Per_Token"] = safe_div_series(by_model["Total_Cost"], by_model["Total_Tokens"])
            by_model["Tokens_Per_$"] = safe_div_series(by_model["Total_Tokens"], by_model["Total_Cost"])

            c1, c2 = st.columns(2)
            with c1:
                fig = px.bar(by_model, x=COL_MODEL, y="Total_Cost", title="Total cost by model ($)")
                fig.update_xaxes(tickangle=-45)
                st.plotly_chart(fig, use_container_width=True)
            with c2:
                fig = px.bar(by_model, x=COL_MODEL, y="Avg_Cost_Per_Token", title="Avg cost per token by model ($/token)")
                fig.update_xaxes(tickangle=-45)
                st.plotly_chart(fig, use_container_width=True)
            fig = px.bar(by_model, x=COL_MODEL, y="Tokens_Per_$", title="Tokens per $ by model")
            fig.update_xaxes(tickangle=-45)
            st.plotly_chart(fig, use_container_width=True)

    with tab3:
        st.subheader("Box plots (per-model distributions)")
        st.caption("Goal: compare spread/median/outliers per model to spot variability and extreme values.")
        if COL_MODEL not in df.columns:
            st.info("No 'Model' column found, skipping per-model box plots.")
        else:
            c1, c2 = st.columns(2)
            with c1:
                fig = px.box(df, x=COL_MODEL, y=COL_COST, title="Cost distribution by model")
                fig.update_xaxes(tickangle=-45)
                st.plotly_chart(fig, use_container_width=True)
            with c2:
                fig = px.box(df, x=COL_MODEL, y=COL_TOKENS, title="Total tokens distribution by model")
                fig.update_xaxes(tickangle=-45)
                st.plotly_chart(fig, use_container_width=True)
            c1, c2 = st.columns(2)
            with c1:
                fig = px.box(df, x=COL_MODEL, y="_cost_per_token", title="Cost per token ($/token) distribution by model")
                fig.update_xaxes(tickangle=-45)
                st.plotly_chart(fig, use_container_width=True)
            with c2:
                fig = px.box(df, x=COL_MODEL, y="_tokens_per_dollar", title="Tokens per $ distribution by model")
                fig.update_xaxes(tickangle=-45)
                st.plotly_chart(fig, use_container_width=True)


def render_clusters(df: pd.DataFrame):
    st.header("Clustering / efficiency")

    if df is None or df.empty:
        st.warning("No data to display. Please select a CSV on the Summary page.")
        return

    if KMeans is None:
        st.info("scikit-learn is not installed, so clustering is disabled. Install it with `pip install scikit-learn` and re-run.")

    df = df.copy()
    df["_cost_per_token"] = safe_div_series(df[COL_COST], df[COL_TOKENS])
    df["_tokens_per_dollar"] = safe_div_series(df[COL_TOKENS], df[COL_COST])
    if COL_CACHE_READ in df.columns:
        df["_cache_ratio"] = safe_div_series(df[COL_CACHE_READ], df[COL_TOKENS])
    else:
        df["_cache_ratio"] = pd.NA

    st.caption("Explore efficiency patterns at request level, model level, and by cache ratio.")

    tabs = st.tabs(["Request level (cost vs tokens)", "Model level (efficiency & scale)", "Cache ratio vs efficiency"])

    # 2) Request level: Cost vs Tokens + clustering
    with tabs[0]:
        st.subheader("Request level: cost vs tokens (idea #2)")
        st.caption("x: tokens, y: cost, color: cluster or model. Helps find high-cost / low-efficiency request groups.")
        req = df[[COL_TOKENS, COL_COST, "_cost_per_token", "_tokens_per_dollar", "_cache_ratio", COL_MODEL]].copy()
        req = req.dropna(subset=[COL_TOKENS, COL_COST])
        req = req[(req[COL_TOKENS] > 0) & (req[COL_COST] > 0)]

        if req.empty:
            st.info("No requests with both Cost and Total Tokens > 0, skipping request-level analysis.")
        else:
            k = st.slider("Number of request clusters (K)", min_value=2, max_value=6, value=3, step=1)
            if KMeans is not None and len(req) >= k:
                feat = req[["_cost_per_token", "_tokens_per_dollar"]].fillna(0.0)
                km = KMeans(n_clusters=k, random_state=42, n_init="auto")
                labels = km.fit_predict(feat.values)
                req["_cluster"] = labels.astype(str)
                color_col = "_cluster"
                color_title = "cluster"
            else:
                req["_cluster"] = req[COL_MODEL].astype(str) if COL_MODEL in req.columns else "all"
                color_col = "_cluster"
                color_title = "model"

            fig = px.scatter(
                req,
                x=COL_TOKENS,
                y=COL_COST,
                color=color_col,
                hover_data=["_cost_per_token", "_tokens_per_dollar", "_cache_ratio", COL_MODEL] if COL_MODEL in req.columns else ["_cost_per_token", "_tokens_per_dollar", "_cache_ratio"],
                labels={COL_TOKENS: "Total Tokens", COL_COST: "Cost ($)", color_col: color_title},
                title="Request-level cost vs tokens (cluster/model)",
            )
            fig.update_traces(marker=dict(size=6, opacity=0.6))
            st.plotly_chart(fig, use_container_width=True)

    # 5) Model level: efficiency & scale clustering
    with tabs[1]:
        st.subheader("Model level: efficiency & scale clusters (idea #5)")
        st.caption("Cluster models by total cost/tokens and unit metrics to see which groups to reduce or expand.")

        if COL_MODEL not in df.columns:
            st.info("No 'Model' column found, skipping model-level analysis.")
        else:
            by_m = df.groupby(COL_MODEL).agg(
                Total_Cost=(COL_COST, "sum"),
                Total_Tokens=(COL_TOKENS, "sum"),
            ).reset_index()
            by_m["Avg_Cost_Per_Token"] = safe_div_series(by_m["Total_Cost"], by_m["Total_Tokens"])
            by_m["Tokens_Per_$"] = safe_div_series(by_m["Total_Tokens"], by_m["Total_Cost"])

            valid = by_m.replace([np.inf, -np.inf], np.nan).dropna(subset=["Total_Cost", "Total_Tokens"])
            if valid.empty:
                st.info("Not enough per-model cost/token data to cluster; skipping.")
            else:
                n_models = len(valid)
                if n_models < 2:
                    st.info("Fewer than 2 valid models; cannot form clusters.")
                    by_m["_cluster"] = "all"
                    k_m = None
                elif n_models == 2:
                    # Streamlit raises if slider(min=max); use a fixed value instead.
                    st.caption("Exactly 2 valid models, so K is fixed to **2**.")
                    k_m = 2
                else:
                    k_m = st.slider(
                        "Number of model clusters (K)",
                        min_value=2,
                        max_value=min(6, n_models),
                        value=min(3, n_models),
                        step=1,
                    )

                if k_m is not None and KMeans is not None and n_models >= k_m:
                    feat_m = valid[["Total_Cost", "Total_Tokens", "Avg_Cost_Per_Token", "Tokens_Per_$"]].fillna(0.0)
                    km_m = KMeans(n_clusters=k_m, random_state=42, n_init="auto")
                    labels_m = km_m.fit_predict(feat_m.values)
                    label_map = dict(zip(valid[COL_MODEL], labels_m.astype(str)))
                    by_m["_cluster"] = by_m[COL_MODEL].map(label_map).fillna("other")
                else:
                    by_m["_cluster"] = "all"

                fig = px.scatter(
                    by_m,
                    x="Avg_Cost_Per_Token",
                    y="Total_Cost",
                    size="Total_Tokens",
                    color="_cluster",
                    hover_data=[COL_MODEL, "Total_Cost", "Total_Tokens", "Tokens_Per_$"],
                    labels={"Avg_Cost_Per_Token": "Avg cost per token ($/token)", "Total_Cost": "Total cost ($)", "_cluster": "cluster"},
                    title="Model-level efficiency & scale scatter (clusters)",
                )
                fig.update_traces(marker=dict(opacity=0.8, line=dict(width=0.5, color="black")))
                fig.update_xaxes(type="log", title="Avg cost per token ($/token, log)")
                fig.update_yaxes(type="log", title="Total cost ($, log)")
                st.plotly_chart(fig, use_container_width=True)

    # 8) Cache ratio vs efficiency
    with tabs[2]:
        st.subheader("Cache ratio vs efficiency (idea #8)")
        st.caption("See how cost_per_token changes as Cache Read ratio increases.")

        if COL_CACHE_READ not in df.columns:
            st.info("No 'Cache Read' column found, skipping cache ratio analysis.")
        else:
            cache_df = df[[COL_TOKENS, COL_COST, "_cost_per_token", "_tokens_per_dollar", "_cache_ratio", COL_MODEL]].copy()
            cache_df = cache_df.dropna(subset=["_cache_ratio", "_cost_per_token"])
            if cache_df.empty:
                st.info("No rows where both cache ratio and cost per token can be computed; cannot plot.")
            else:
                fig = px.scatter(
                    cache_df,
                    x="_cache_ratio",
                    y="_cost_per_token",
                    color=COL_MODEL if COL_MODEL in cache_df.columns else None,
                    hover_data=[COL_TOKENS, COL_COST, "_tokens_per_dollar"],
                    labels={"_cache_ratio": "Cache ratio (Cache Read / Total Tokens)", "_cost_per_token": "Cost per token ($/token)"},
                    title="Cache ratio vs cost per token",
                )
                fig.update_traces(marker=dict(size=6, opacity=0.6))
                st.plotly_chart(fig, use_container_width=True)


def render_timeseries(df: pd.DataFrame):
    st.header("Timeseries")

    if df is None or df.empty:
        st.warning("No data to display. Please select a CSV on the Summary page.")
        return

    if COL_DATE not in df.columns:
        st.info("No 'Date' column found, skipping timeseries analysis.")
        return

    ts = df.copy()
    # Parse as UTC then drop tz info to keep a tz-naive index.
    ts[COL_DATE] = pd.to_datetime(ts[COL_DATE], errors="coerce", utc=True)
    ts = ts.dropna(subset=[COL_DATE])
    if ts.empty:
        st.info("No rows with parseable Date values; skipping timeseries analysis.")
        return

    ts = ts.sort_values(COL_DATE)
    ts = ts.set_index(COL_DATE)
    # Convert DatetimeIndex to tz-naive to avoid slicing comparison errors.
    if ts.index.tz is not None:
        ts.index = ts.index.tz_convert(None)

    # Resampling interval / analysis range / model filter
    freq_label_map = {
        "5 min": "5T",
        "15 min": "15T",
        "30 min": "30T",
        "60 min": "60T",
    }
    col_ctrl1, col_ctrl2 = st.columns(2)
    with col_ctrl1:
        freq_label = st.selectbox("Resampling interval", list(freq_label_map.keys()), index=1)
    freq = freq_label_map[freq_label]

    # Compute default start/end from full range
    full_start = ts.index.min().date()
    full_end = ts.index.max().date()

    col_range1, col_range2 = st.columns(2)
    with col_range1:
        date_range = st.date_input(
            "Analysis range (start ~ end)",
            value=(full_start, full_end),
            min_value=full_start,
            max_value=full_end,
        )
    # Streamlit can return a single date or a 1/2-element tuple; handle safely.
    if isinstance(date_range, tuple):
        if len(date_range) == 2:
            start_date, end_date = date_range
        elif len(date_range) == 1:
            start_date = end_date = date_range[0]
        else:
            start_date = end_date = full_start
    else:
        start_date = end_date = date_range

    # Model filter
    models: list[str] = []
    if COL_MODEL in ts.columns:
        models = sorted([m for m in ts[COL_MODEL].dropna().unique().tolist() if m != ""])
    with col_ctrl2:
        model_choice: Optional[str] = None
        if models:
            model_choice = st.selectbox("Model filter", ["All"] + models, index=0)

    # Apply range/model filters using date comparisons (inclusive).
    mask = (ts.index.date >= start_date) & (ts.index.date <= end_date)
    ts_view = ts[mask]
    if model_choice and model_choice != "All":
        ts_view = ts_view[ts_view[COL_MODEL] == model_choice]

    if ts_view.empty:
        st.info(
            "No data matches the selected range/model filters. "
            "Reverting to full range (all models) for the timeseries charts."
        )
        ts_view = ts

    # Aggregate via resampling (sum + count)
    agg = ts_view.resample(freq).agg(
        {
            COL_COST: "sum",
            COL_TOKENS: "sum",
        }
    )
    agg["Requests"] = ts_view[COL_TOKENS].resample(freq).count()
    agg["Avg_Cost_Per_Token"] = safe_div_series(agg[COL_COST], agg[COL_TOKENS])
    agg["Tokens_Per_$"] = safe_div_series(agg[COL_TOKENS], agg[COL_COST])
    agg = agg.dropna(how="all")

    if agg.empty:
        st.info("Too little data at the selected interval to build a timeseries.")
        return

    st.caption(
        f"Resample at {freq_label} to track cost, tokens, requests, and unit metrics over time."
        + ("" if not model_choice or model_choice == "All" else f" (model: {model_choice})")
    )

    tab1, tab2, tab3 = st.tabs(["Scale", "Efficiency", "Cumulative"])

    with tab1:
        st.subheader("Total cost / total tokens / request count")
        c1, c2 = st.columns(2)
        with c1:
            fig = px.line(agg, y=COL_COST, title="Total cost ($)")
            fig.update_yaxes(title="Total cost ($)")
            st.plotly_chart(fig, use_container_width=True)
        with c2:
            fig = px.line(agg, y=COL_TOKENS, title="Total tokens")
            fig.update_yaxes(title="Total tokens")
            st.plotly_chart(fig, use_container_width=True)
        fig = px.line(agg, y="Requests", title="Requests")
        fig.update_yaxes(title="Requests")
        st.plotly_chart(fig, use_container_width=True)

    with tab2:
        st.subheader("Efficiency over time")
        fig = px.line(
            agg,
            y=["Avg_Cost_Per_Token", "Tokens_Per_$"],
            title="Avg cost per token / tokens per $",
            labels={"value": "value", "variable": "metric"},
        )
        st.plotly_chart(fig, use_container_width=True)

    with tab3:
        st.subheader("Cumulative cost / cumulative tokens")
        st.caption("Because the scales differ, cost and tokens are shown separately.")
        cum = agg[[COL_COST, COL_TOKENS]].fillna(0).cumsum()
        c1, c2 = st.columns(2)
        with c1:
            fig = px.line(cum, y=COL_COST, title="Cumulative cost ($)")
            fig.update_yaxes(title="Cumulative cost ($)")
            st.plotly_chart(fig, use_container_width=True)
        with c2:
            fig = px.line(cum, y=COL_TOKENS, title="Cumulative tokens")
            fig.update_yaxes(title="Cumulative tokens")
            st.plotly_chart(fig, use_container_width=True)


def main():
    st.set_page_config(page_title="Token usage analysis", layout="wide")
    st.sidebar.title("Token usage & cost")

    df, path, csv_files = ensure_data()
    if not csv_files:
        st.warning("No CSV files found in the analyze folder.")
        return

    if not st.session_state.get("selected_csv") or st.session_state["selected_csv"] not in csv_files:
        st.session_state["selected_csv"] = csv_files[0]
    idx = csv_files.index(st.session_state["selected_csv"])
    selected = st.sidebar.selectbox(
        "Dataset",
        csv_files,
        index=idx,
        key="csv_select",
        help="Select the CSV file to analyze. Summary/charts will be based on the selected dataset.",
    )
    st.session_state["selected_csv"] = selected
    path = os.path.join(os.path.dirname(os.path.abspath(__file__)), selected)
    df = load_csv(path)

    page = st.sidebar.radio(
        "Page",
        ["Summary", "Charts", "Clustering / efficiency", "Timeseries"],
        label_visibility="collapsed",
    )
    st.sidebar.caption(
        "**Summary**: scale → summary table → by model\n"
        "**Charts**: distributions → model comparison → box plots\n"
        "**Clustering/efficiency**: clusters by request/model/cache ratio\n"
        "**Timeseries**: 5/15/30/60-min resampling trends"
    )

    st.title("Token usage & cost analysis")
    st.caption(
        f"**Current dataset**: `{selected}` · Flow: scale → summary stats (spread/range) → model segment → charts for patterns & outliers"
    )

    if page == "Summary":
        if df is not None and not df.empty:
            render_summary(df)
        else:
            st.warning("No data available or failed to load.")
    elif page == "Charts":
        render_graphs(df)
    elif page == "Clustering / efficiency":
        render_clusters(df)
    else:
        render_timeseries(df)


if __name__ == "__main__":
    main()
