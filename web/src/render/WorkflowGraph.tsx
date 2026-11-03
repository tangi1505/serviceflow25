// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The workflow graph (UI26.2, E26, E27, E31): the states of a type as nodes, every from
// state of a transition as an edge, drawn with React Flow above the tables of the workflow
// editor. The graph is for looking, checking and arranging: a click on an edge marks the
// whole transition in the graph and in the table; a connection dragged between two states
// prefills the form of a new transition; nodes move with the mouse or the arrow keys, and
// "Anordnen" lets dagre arrange them. Moving changes only the screen until "Anordnung
// speichern" writes the whole layout as one change of the customer layer (field layout of
// the type, E27). Without a saved layout dagre arranges on every opening. Whether the graph
// is open is kept per account in localStorage.
import dagre from "@dagrejs/dagre";
import {
  applyNodeChanges,
  Background,
  ConnectionMode,
  Controls,
  Handle,
  MarkerType,
  Position,
  ReactFlow,
  type Connection,
  type Edge,
  type EdgeChange,
  type Node,
  type NodeChange,
  type NodeProps,
  type NodeTypes,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
} from "react";
import { useTranslation } from "react-i18next";

import { labelOf, type State, type Transition, type TypeDefinition } from "@/api/client";
import { Button } from "@/ui/button";

import type { Apply } from "./CustomizePage";

const nodeWidth = 170;
const nodeHeight = 46;
const own = (name: string) => name.startsWith("k_");

type StateData = { label: string; name: string; initial: boolean; own: boolean; sub: string };
type StateNode = Node<StateData, "state">;

function StateNodeView({ data }: NodeProps<StateNode>) {
  return (
    <>
      <Handle type="target" position={Position.Top} />
      <div className="flex flex-col px-3 py-1 text-sm leading-tight">
        <span className="font-medium">{data.label}</span>
        <span className="text-xs text-muted-foreground">{data.sub}</span>
      </div>
      <Handle type="source" position={Position.Bottom} />
    </>
  );
}

const nodeTypes: NodeTypes = { state: StateNodeView };

// arranged lays the states out top-down with dagre; the result is the position of every
// state's top-left corner.
function arranged(
  states: State[],
  transitions: Transition[],
): Record<string, { x: number; y: number }> {
  const g = new dagre.graphlib.Graph();
  g.setDefaultEdgeLabel(() => ({}));
  g.setGraph({ rankdir: "TB", nodesep: 40, ranksep: 70 });
  for (const s of states) {
    g.setNode(s.name, { width: nodeWidth, height: nodeHeight });
  }
  for (const tr of transitions) {
    for (const from of tr.from) {
      if (from !== tr.to) {
        g.setEdge(from, tr.to);
      }
    }
  }
  dagre.layout(g);
  const out: Record<string, { x: number; y: number }> = {};
  for (const s of states) {
    const n = g.node(s.name);
    out[s.name] = { x: Math.round(n.x - nodeWidth / 2), y: Math.round(n.y - nodeHeight / 2) };
  }
  return out;
}

// Colours of the graph from the tokens of the interface (UI29 brings tokens.css).
const theme = {
  "--xy-background-color": "var(--background)",
  "--xy-node-background-color": "var(--card)",
  "--xy-node-color": "var(--foreground)",
  "--xy-node-border": "1px solid var(--input)",
  "--xy-node-border-radius": "8px",
  "--xy-node-boxshadow-selected": "0 0 0 2px var(--ring)",
  "--xy-edge-stroke": "var(--muted-foreground)",
  "--xy-edge-stroke-selected": "var(--primary)",
  "--xy-edge-label-background-color": "var(--card)",
  "--xy-edge-label-color": "var(--foreground)",
  "--xy-handle-background-color": "var(--muted-foreground)",
  "--xy-handle-border-color": "var(--card)",
  "--xy-controls-button-background-color": "var(--card)",
  "--xy-controls-button-color": "var(--foreground)",
  "--xy-controls-button-border-color": "var(--border)",
  "--xy-attribution-background-color": "var(--card)",
} as CSSProperties;

export function WorkflowGraph({
  type,
  selected,
  onSelect,
  onConnect,
  busy,
  apply,
  storageKey,
}: {
  type: TypeDefinition;
  // selected is the transition marked in graph and table, by name.
  selected: string | null;
  onSelect: (name: string | null) => void;
  // onConnect is a connection dragged from one state to another.
  onConnect: (from: string, to: string) => void;
  busy: boolean;
  apply: Apply;
  storageKey: string;
}) {
  const { t, i18n } = useTranslation();
  const id = useId();
  const states = useMemo(() => type.states ?? [], [type.states]);
  const transitions = useMemo(() => type.transitions ?? [], [type.transitions]);
  const [open, setOpen] = useState(() => {
    try {
      return localStorage.getItem(storageKey) !== "closed";
    } catch {
      return true;
    }
  });
  const toggle = () => {
    const next = !open;
    setOpen(next);
    try {
      localStorage.setItem(storageKey, next ? "open" : "closed");
    } catch {
      // Without storage the choice holds for this page only.
    }
  };

  // The positions: the saved layout where it has the state, dagre for the rest.
  const layoutKey = JSON.stringify(type.layout ?? null);
  const stateKey = states.map((s) => s.name).join(",");
  const positions = useMemo(() => {
    const auto = arranged(states, transitions);
    const saved = type.layout ?? {};
    const out: Record<string, { x: number; y: number }> = {};
    for (const s of states) {
      out[s.name] = saved[s.name] ?? auto[s.name]!;
    }
    return out;
    // Recomputed when the states or the saved layout change, not on every label edit.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [stateKey, layoutKey]);

  const toNode = useCallback(
    (s: State, position: { x: number; y: number }): StateNode => {
      const label = labelOf(s.label, i18n.language, s.name);
      const flags = [s.name];
      if (s.initial) {
        flags.push(t("workflow.graph.initial"));
      }
      return {
        id: s.name,
        type: "state",
        position,
        width: nodeWidth,
        height: nodeHeight,
        ariaLabel: t("workflow.graph.nodeLabel", { name: label }),
        className: [
          // A custom node brings no look of its own: card, border, corners here.
          "rounded-md border bg-card shadow-sm",
          s.initial
            ? "border-2 border-foreground"
            : own(s.name)
              ? "border-dashed border-input"
              : "border-input",
        ].join(" "),
        data: {
          label,
          name: s.name,
          initial: s.initial === true,
          own: own(s.name),
          sub: flags.join(" · "),
        },
      };
    },
    [i18n.language, t],
  );

  const [nodes, setNodes] = useState<StateNode[]>(() =>
    states.map((s) => toNode(s, positions[s.name]!)),
  );
  const [dirty, setDirty] = useState(false);
  const seenLayout = useRef(layoutKey);
  useEffect(() => {
    const layoutChanged = seenLayout.current !== layoutKey;
    seenLayout.current = layoutKey;
    setNodes((prev) =>
      states.map((s) => {
        const before = prev.find((n) => n.id === s.name);
        const position = layoutChanged || !before ? positions[s.name]! : before.position;
        return { ...toNode(s, position), selected: before?.selected };
      }),
    );
    if (layoutChanged) {
      setDirty(false);
    }
  }, [states, positions, layoutKey, toNode]);

  // Leaving with moved nodes is pointed out by the browser.
  useEffect(() => {
    if (!dirty) {
      return;
    }
    const warn = (e: BeforeUnloadEvent) => {
      e.preventDefault();
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);

  const onNodesChange = useCallback((changes: NodeChange<StateNode>[]) => {
    setNodes((prev) => applyNodeChanges(changes, prev));
    if (changes.some((c) => c.type === "position" && c.position !== undefined)) {
      setDirty(true);
    }
  }, []);

  const transitionOf = (edgeId: string) => edgeId.slice(0, edgeId.lastIndexOf(":"));
  const onEdgesChange = useCallback(
    (changes: EdgeChange<Edge>[]) => {
      const picked = changes.find((c) => c.type === "select" && c.selected);
      if (picked && picked.type === "select") {
        onSelect(transitionOf(picked.id));
      } else if (changes.some((c) => c.type === "select")) {
        onSelect(null);
      }
    },
    [onSelect],
  );

  const stateLabel = (name: string) => {
    const s = states.find((x) => x.name === name);
    return s ? labelOf(s.label, i18n.language, s.name) : name;
  };
  const edges = useMemo<Edge[]>(
    () =>
      transitions.flatMap((tr) => {
        const label = labelOf(tr.label, i18n.language, tr.name);
        const isSelected = tr.name === selected;
        const stroke = isSelected
          ? "var(--primary)"
          : tr.disabled
            ? "var(--muted-foreground)"
            : "var(--foreground)";
        return tr.from.map((from): Edge => ({
          id: `${tr.name}:${from}`,
          source: from,
          target: tr.to,
          label: tr.disabled ? `${label} (${t("workflow.flag.disabled")})` : label,
          ariaLabel: t("workflow.graph.edgeLabel", {
            name: label,
            from: stateLabel(from),
            to: stateLabel(tr.to),
          }),
          selected: isSelected,
          style: {
            stroke,
            strokeWidth: isSelected ? 2.5 : 1.5,
            strokeDasharray: tr.disabled ? "6 4" : undefined,
          },
          labelStyle: { fill: stroke, fontWeight: isSelected ? 600 : 400 },
          markerEnd: { type: MarkerType.ArrowClosed, color: stroke, width: 18, height: 18 },
        }));
      }),
    // stateLabel reads states, which are part of transitions' key below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [transitions, states, selected, i18n.language, t],
  );

  const connect = (c: Connection) => {
    if (c.source && c.target && c.source !== c.target) {
      onConnect(c.source, c.target);
    }
  };
  const arrange = () => {
    const auto = arranged(states, transitions);
    setNodes((prev) => prev.map((n) => ({ ...n, position: auto[n.id] ?? n.position })));
    setDirty(true);
  };
  const save = () => {
    const layout: Record<string, { x: number; y: number }> = {};
    for (const n of nodes) {
      layout[n.id] = { x: Math.round(n.position.x), y: Math.round(n.position.y) };
    }
    apply({ op: "set_layout", field: "layout", layout }, t("workflow.graph.saved"));
  };

  const ariaLabelConfig = {
    "node.a11yDescription.default": t("workflow.graph.a11y.node"),
    "node.a11yDescription.keyboardDisabled": t("workflow.graph.a11y.node"),
    "node.a11yDescription.ariaLiveMessage": ({ x, y }: { x: number; y: number }) =>
      t("workflow.graph.a11y.live", { x: Math.round(x), y: Math.round(y) }),
    "edge.a11yDescription.default": t("workflow.graph.a11y.edge"),
    "controls.ariaLabel": t("workflow.graph.a11y.controls"),
    "controls.zoomIn.ariaLabel": t("workflow.graph.a11y.zoomIn"),
    "controls.zoomOut.ariaLabel": t("workflow.graph.a11y.zoomOut"),
    "controls.fitView.ariaLabel": t("workflow.graph.a11y.fitView"),
  };

  if (states.length === 0) {
    return null;
  }
  return (
    <section aria-labelledby={`${id}-h`} className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id={`${id}-h`} className="text-lg font-semibold">
          {t("workflow.graph.title")}
        </h2>
        <div className="flex flex-wrap items-center gap-2">
          {open && dirty && (
            <span className="text-sm text-muted-foreground" role="status">
              {t("workflow.graph.unsaved")}
            </span>
          )}
          {open && (
            <Button variant="outline" size="sm" onClick={arrange}>
              {t("workflow.graph.arrange")}
            </Button>
          )}
          {open && (
            <Button size="sm" disabled={busy || !dirty} onClick={save}>
              {t("workflow.graph.save")}
            </Button>
          )}
          <Button
            variant="ghost"
            size="sm"
            aria-expanded={open}
            aria-controls={`${id}-g`}
            onClick={toggle}
          >
            {open ? t("workflow.graph.collapse") : t("workflow.graph.expand")}
          </Button>
        </div>
      </div>
      {open && (
        <>
          <div
            id={`${id}-g`}
            className="h-96 overflow-hidden rounded-md border"
            style={theme}
            data-testid="workflow-graph"
          >
            <ReactFlow
              nodes={nodes}
              edges={edges}
              nodeTypes={nodeTypes}
              onNodesChange={onNodesChange}
              onEdgesChange={onEdgesChange}
              onConnect={connect}
              onPaneClick={() => onSelect(null)}
              connectionMode={ConnectionMode.Loose}
              fitView
              minZoom={0.3}
              maxZoom={2}
              nodesConnectable={!busy}
              edgesFocusable
              deleteKeyCode={null}
              // The link to the maker (grey on white, 2.8:1, and a link out of a VS-NfD system) is
              // hidden, as the MIT licence allows; React Flow stands in README and licences.
              proOptions={{ hideAttribution: true }}
              ariaLabelConfig={ariaLabelConfig}
            >
              <Background gap={16} />
              <Controls showInteractive={false} />
            </ReactFlow>
          </div>
          <p className="text-xs text-muted-foreground">{t("workflow.graph.legend")}</p>
        </>
      )}
    </section>
  );
}
