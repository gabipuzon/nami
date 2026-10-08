"use client";

import { recordPerformance } from "../lib/performance";

import { memo, useId } from "react";
import { BaseEdge, getBezierPath, getStraightPath, getSmoothStepPath, type EdgeProps } from "@xyflow/react";

export const DependencyEdge = memo(function DependencyEdge({ sourceX, sourceY, sourcePosition, targetX, targetY, targetPosition, style, interactionWidth, data }: EdgeProps) {
  recordPerformance("render.DependencyEdge");
  const gradientID = `nami-line-${useId().replaceAll(":", "")}`;
  const coordinates = { sourceX, sourceY, sourcePosition, targetX, targetY, targetPosition };
  const [path, labelX, labelY] = data?.lineStyle === "straight" ? getStraightPath(coordinates)
    : data?.lineStyle === "stepped" ? getSmoothStepPath({ ...coordinates, borderRadius: 0 })
    : getBezierPath(coordinates);
  return <>
    <defs>
      <linearGradient id={gradientID} gradientUnits="userSpaceOnUse" x1={sourceX} y1={sourceY} x2={targetX} y2={targetY}>
        <stop offset="0%" style={{ stopColor: "var(--export-line)" }} />
        <stop offset="100%" style={{ stopColor: "var(--import-line)" }} />
      </linearGradient>
    </defs>
    <BaseEdge path={path} interactionWidth={interactionWidth} style={{ ...style, stroke: `url(#${gradientID})` }} />
    {typeof data?.bundleCount === "number" && <text x={labelX} y={labelY} className="bundle-count" textAnchor="middle">{data.bundleCount}</text>}
  </>;
});
