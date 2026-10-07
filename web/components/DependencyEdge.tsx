"use client";

import { useId } from "react";
import { BaseEdge, getBezierPath, type EdgeProps } from "@xyflow/react";

export function DependencyEdge({ sourceX, sourceY, sourcePosition, targetX, targetY, targetPosition, style, interactionWidth }: EdgeProps) {
  const gradientID = `nami-line-${useId().replaceAll(":", "")}`;
  const [path] = getBezierPath({ sourceX, sourceY, sourcePosition, targetX, targetY, targetPosition });
  return <>
    <defs>
      <linearGradient id={gradientID} gradientUnits="userSpaceOnUse" x1={sourceX} y1={sourceY} x2={targetX} y2={targetY}>
        <stop offset="0%" style={{ stopColor: "var(--export-line)" }} />
        <stop offset="100%" style={{ stopColor: "var(--import-line)" }} />
      </linearGradient>
    </defs>
    <BaseEdge path={path} interactionWidth={interactionWidth} style={{ ...style, stroke: `url(#${gradientID})` }} />
  </>;
}
