// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The name of a referenced record (UI3): a person by first and last name, anything else by
// the title field of its type. A record the session may not read shows its type and a short id.
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import {
  labelOf,
  qualifiedName,
  recordQuery,
  typesQuery,
  type ActorRef,
  type RecordData,
  type TypeDefinition,
} from "@/api/client";

export function titleOf(type: TypeDefinition | undefined, rec: RecordData): string {
  // A person, also while the types are loading: first and last name.
  if (!type || qualifiedName(type) === "core.person") {
    const name = [rec["first_name"], rec["last_name"]].filter(Boolean).join(" ");
    if (name) {
      return name;
    }
  }
  const field = type?.title_field ?? type?.fields[0]?.name ?? "id";
  return String(rec[field] ?? rec["id"] ?? "");
}

export function RefName({ refTo, link = false }: { refTo: ActorRef; link?: boolean }) {
  const { i18n } = useTranslation();
  const types = useQuery(typesQuery);
  const rec = useQuery({ ...recordQuery(refTo.type, refTo.id), retry: false });
  const type = types.data?.find((t) => qualifiedName(t) === refTo.type);
  if (!rec.data) {
    const typeLabel = labelOf(type?.label, i18n.language, refTo.type);
    return (
      <span className="text-muted-foreground">
        {typeLabel} {refTo.id.slice(-6)}
      </span>
    );
  }
  const name = titleOf(type, rec.data);
  if (!link) {
    return <span>{name}</span>;
  }
  return (
    <Link
      to="/types/$typeName/$id"
      params={{ typeName: refTo.type, id: refTo.id }}
      className="underline"
    >
      {name}
    </Link>
  );
}
