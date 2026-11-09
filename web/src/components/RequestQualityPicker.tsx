import { useId } from "react";
import type { RequestProfileOption, RequestProfileOptions } from "@/api/types";
import { cn } from "@/lib/utils";

/** The profile a request is sent with: which server it came from, and its id. */
export interface QualityProfileChoice {
  connectionId: string;
  profileId: number;
}

/** A profile from one server's list, with the server it belongs to. */
export interface FlattenedProfile extends RequestProfileOption {
  connectionId: string;
  serverName: string;
}

/**
 * Every profile the picker may offer, in server order. A profile id means
 * nothing across servers, so the connection always travels with it.
 */
export function flattenProfiles(options: RequestProfileOptions | undefined): FlattenedProfile[] {
  const out: FlattenedProfile[] = [];
  for (const server of options?.servers ?? []) {
    for (const profile of server.profiles) {
      out.push({
        ...profile,
        connectionId: server.connection_id,
        serverName: server.server_name,
      });
    }
  }
  return out;
}

/** Whether the picker is worth showing: requesters may choose, and there is a choice to make. */
export function qualityPickerAvailable(options: RequestProfileOptions | undefined): boolean {
  return Boolean(options?.enabled) && flattenProfiles(options).length > 1;
}

/**
 * The profile the server would use with no choice: the one marked default, or
 * the first. Used only for the select's starting label — a request that never
 * touches the picker sends no profile at all, so the server keeps its own.
 */
export function defaultProfileChoice(
  options: RequestProfileOptions | undefined,
): FlattenedProfile | null {
  const profiles = flattenProfiles(options);
  return profiles.find((profile) => profile.default) ?? profiles[0] ?? null;
}

function choiceKey(choice: QualityProfileChoice): string {
  return `${choice.connectionId}:${choice.profileId}`;
}

function profileKey(profile: FlattenedProfile): string {
  return `${profile.connectionId}:${profile.id}`;
}

/**
 * The quality profile a request downloads with. The select starts on the
 * server's own profile; picking another sends it with the request, and leaves
 * the server's default untouched.
 */
export default function RequestQualityPicker({
  options,
  value,
  onChange,
  className,
  disabled,
}: {
  options: RequestProfileOptions | undefined;
  value: QualityProfileChoice | null;
  onChange: (choice: QualityProfileChoice | null) => void;
  className?: string;
  disabled?: boolean;
}) {
  const id = useId();
  const profiles = flattenProfiles(options);
  if (!qualityPickerAvailable(options)) return null;
  const fallback = defaultProfileChoice(options);
  const selected = value ? choiceKey(value) : fallback ? profileKey(fallback) : "";
  const servers = options?.servers ?? [];
  const multiServer = servers.length > 1;

  return (
    <div className={cn("space-y-1.5", className)}>
      <label htmlFor={id} className="text-sm font-medium">
        Quality profile
      </label>
      <select
        id={id}
        className="border-input bg-background focus-visible:ring-ring h-9 w-full rounded-md border px-3 text-sm focus-visible:ring-2 focus-visible:outline-none"
        value={selected}
        disabled={disabled}
        onChange={(event) => {
          const next = profiles.find((profile) => profileKey(profile) === event.target.value);
          onChange(next ? { connectionId: next.connectionId, profileId: next.id } : null);
        }}
      >
        {servers.map((server) => (
          <optgroup key={server.connection_id} label={server.server_name}>
            {server.profiles.map((profile) => {
              const flattened: FlattenedProfile = {
                ...profile,
                connectionId: server.connection_id,
                serverName: server.server_name,
              };
              return (
                <option key={profileKey(flattened)} value={profileKey(flattened)}>
                  {profile.name}
                  {multiServer ? ` — ${server.server_name}` : ""}
                </option>
              );
            })}
          </optgroup>
        ))}
      </select>
      <p className="text-muted-foreground text-xs">
        The server downloads with this profile for your request only; its own default stays as
        it is.
      </p>
    </div>
  );
}
