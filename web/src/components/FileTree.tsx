import { ChevronRight, File as FileIcon, Folder, FolderOpen } from "lucide-react";
import { useMemo, useState } from "react";
import { formatBytes } from "../lib/format";
import { cn } from "../lib/cn";

// One node of the rebuilt tree. `children` present = directory; absent = file.
type Node = {
  name: string;
  path: string;
  size: number;
  count: number;
  children?: Node[];
};

type Dir = {
  size: number;
  count: number;
  dirs: Map<string, Dir>;
  files: { name: string; size: number }[];
};

// The API sends flat slash-joined paths — rebuilding the tree here keeps the
// payload small and lets the sizes/counts roll up in one pass.
function buildTree(files: [string, number][]): Node[] {
  const root: Dir = { size: 0, count: 0, dirs: new Map(), files: [] };
  for (const [path, size] of files) {
    const parts = path.split("/");
    let dir = root;
    dir.size += size;
    dir.count += 1;
    for (let index = 0; index < parts.length - 1; index++) {
      let next = dir.dirs.get(parts[index]);
      if (!next) {
        next = { size: 0, count: 0, dirs: new Map(), files: [] };
        dir.dirs.set(parts[index], next);
      }
      next.size += size;
      next.count += 1;
      dir = next;
    }
    dir.files.push({ name: parts[parts.length - 1], size });
  }
  return toNodes(root, "");
}

// Directories first, then files, each by name: the dump's own order is lost
// in parsing, and a predictable one beats a half-kept one.
function toNodes(dir: Dir, prefix: string): Node[] {
  const byName = (left: { name: string }, right: { name: string }) =>
    left.name.localeCompare(right.name, undefined, { numeric: true, sensitivity: "base" });
  const dirs: Node[] = [...dir.dirs.entries()]
    .map(([name, dir]) => ({
      name,
      path: prefix + name + "/",
      size: dir.size,
      count: dir.count,
      children: toNodes(dir, prefix + name + "/"),
    }))
    .sort(byName);
  const files: Node[] = dir.files
    .map((file) => ({ name: file.name, path: prefix + file.name, size: file.size, count: 1 }))
    .sort(byName);
  return [...dirs, ...files];
}

// Above this many files the tree opens collapsed past the first level —
// a 900-entry season pack would otherwise bury the whole drawer.
const AUTO_EXPAND_LIMIT = 40;

export function FileTree({ files }: { files: [string, number][] }) {
  const nodes = useMemo(() => buildTree(files), [files]);
  const [collapsed, setCollapsed] = useState<Set<string>>(() => {
    if (files.length <= AUTO_EXPAND_LIMIT) return new Set();
    const shut = new Set<string>();
    const walk = (ns: Node[], depth: number) => {
      for (const node of ns) {
        if (!node.children) continue;
        if (depth > 0) shut.add(node.path);
        walk(node.children, depth + 1);
      }
    };
    walk(nodes, 0);
    return shut;
  });

  const toggle = (path: string) =>
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (!next.delete(path)) next.add(path);
      return next;
    });

  return (
    // Long listings scroll inside the section instead of stretching the drawer;
    // the drawer's own scroll then still reaches the description below.
    <div className="max-h-[420px] overflow-y-auto -mx-1 px-1">
      <Level nodes={nodes} depth={0} collapsed={collapsed} onToggle={toggle} />
    </div>
  );
}

function Level({
  nodes,
  depth,
  collapsed,
  onToggle,
}: {
  nodes: Node[];
  depth: number;
  collapsed: Set<string>;
  onToggle: (path: string) => void;
}) {
  return (
    <ul>
      {nodes.map((node) =>
        node.children ? (
          <li key={node.path}>
            <button
              type="button"
              onClick={() => onToggle(node.path)}
              aria-expanded={!collapsed.has(node.path)}
              className={cn(
                "w-full flex items-center gap-1.5 py-1 text-left rounded-sm",
                "hover:text-[var(--color-accent)] transition-colors cursor-pointer"
              )}
              style={{ paddingLeft: depth * 14 }}
            >
              <ChevronRight
                className={cn(
                  "size-3 shrink-0 transition-transform",
                  !collapsed.has(node.path) && "rotate-90"
                )}
              />
              {collapsed.has(node.path) ? (
                <Folder className="size-3.5 shrink-0 text-[var(--color-ink-muted)]" />
              ) : (
                <FolderOpen className="size-3.5 shrink-0 text-[var(--color-ink-muted)]" />
              )}
              <span className="min-w-0 truncate text-[12.5px] font-medium">{node.name}</span>
              <span className="ml-auto pl-3 shrink-0 flex items-baseline gap-2 text-[11px] tabular-nums text-[var(--color-ink-muted)]">
                <span>{node.count}</span>
                <span>{formatBytes(node.size)}</span>
              </span>
            </button>
            {!collapsed.has(node.path) && (
              <Level nodes={node.children} depth={depth + 1} collapsed={collapsed} onToggle={onToggle} />
            )}
          </li>
        ) : (
          <li
            key={node.path}
            className="flex items-center gap-1.5 py-1"
            // The chevron's width, so files line up with the folder names
            // above them instead of with their chevrons.
            style={{ paddingLeft: depth * 14 + 18 }}
          >
            <FileIcon className="size-3.5 shrink-0 text-[var(--color-rule)]" />
            <span className="min-w-0 truncate text-[12.5px] text-[var(--color-ink-soft)]">
              {node.name}
            </span>
            <span className="ml-auto pl-3 shrink-0 text-[11px] tabular-nums text-[var(--color-ink-muted)]">
              {formatBytes(node.size)}
            </span>
          </li>
        )
      )}
    </ul>
  );
}
