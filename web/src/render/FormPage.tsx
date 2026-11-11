// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// A form in the workplace (SD9): a note the service desk takes, such as the phone note, filled
// in like a form of the portal; the new record opens afterwards. The menu "Neu" of a list and
// the command line lead here.
import { queryOptions, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { api, labelOf, qualifiedName, typesQuery } from "@/api/client";
import { FormFill } from "@/portal/FormFill";

// workplaceFormsQuery are the forms offered in the workplace (SD9).
export const workplaceFormsQuery = queryOptions({
  queryKey: ["forms", "workplace"],
  queryFn: () => api.forms("workplace"),
  staleTime: 60_000,
});

export function WorkplaceFormPage() {
  const { i18n } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { formId } = useParams({ from: "/shell/forms/$formId" });
  const forms = useQuery(workplaceFormsQuery);
  const types = useQuery(typesQuery);
  const typeName = forms.data?.find((f) => f.id === formId)?.target_type;
  const target = types.data?.find((x) => qualifiedName(x) === typeName);
  return (
    <div className="flex flex-col gap-4 p-4">
      <FormFill
        formId={formId}
        crumb={
          typeName && (
            <Link to="/types/$typeName" params={{ typeName }} className="underline">
              {labelOf(target?.label, i18n.language, typeName)}
            </Link>
          )
        }
        onSaved={(saved, t) => {
          const name = qualifiedName(t);
          void queryClient.invalidateQueries({ queryKey: ["records", name] });
          void navigate({
            to: "/types/$typeName/$id",
            params: { typeName: name, id: String(saved["id"]) },
          });
        }}
        onCancel={() =>
          void (typeName
            ? navigate({ to: "/types/$typeName", params: { typeName } })
            : navigate({ to: "/" }))
        }
      />
    </div>
  );
}
