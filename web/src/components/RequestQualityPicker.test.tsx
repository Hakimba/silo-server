import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { RequestProfileOptions } from "@/api/types";
import RequestQualityPicker, {
  defaultProfileChoice,
  flattenProfiles,
  qualityPickerAvailable,
} from "./RequestQualityPicker";

const twoProfiles: RequestProfileOptions = {
  enabled: true,
  servers: [
    {
      connection_id: "c-radarr",
      server_name: "Radarr",
      profiles: [
        { id: 3, name: "French HD 1080p", is_4k: false, default: true },
        { id: 4, name: "French 4K Remux", is_4k: true, default: false },
      ],
    },
  ],
};

describe("RequestQualityPicker", () => {
  it("is offered only when there is a choice to make", () => {
    expect(qualityPickerAvailable(twoProfiles)).toBe(true);
    expect(qualityPickerAvailable({ enabled: false, servers: twoProfiles.servers })).toBe(false);
    expect(
      qualityPickerAvailable({
        enabled: true,
        servers: [{ ...twoProfiles.servers[0]!, profiles: [twoProfiles.servers[0]!.profiles[0]!] }],
      }),
    ).toBe(false);
    expect(qualityPickerAvailable(undefined)).toBe(false);
  });

  it("carries the server with the profile, since a profile id is per server", () => {
    const flattened = flattenProfiles({
      enabled: true,
      servers: [
        twoProfiles.servers[0]!,
        {
          connection_id: "c-sonarr",
          server_name: "Sonarr",
          profiles: [{ id: 3, name: "Other 1080p", is_4k: false, default: true }],
        },
      ],
    });
    expect(flattened.map((profile) => `${profile.connectionId}:${profile.id}`)).toEqual([
      "c-radarr:3",
      "c-radarr:4",
      "c-sonarr:3",
    ]);
  });

  it("starts on the server's own profile", () => {
    expect(defaultProfileChoice(twoProfiles)).toMatchObject({ id: 3, connectionId: "c-radarr" });
  });

  it("offers the profiles and reports the pick", () => {
    const onChange = vi.fn();
    render(<RequestQualityPicker options={twoProfiles} value={null} onChange={onChange} />);
    const select = screen.getByLabelText("Quality profile");
    expect(select).toHaveValue("c-radarr:3");
    expect(screen.getByRole("option", { name: "French 4K Remux (4K)" })).toBeInTheDocument();
    fireEvent.change(select, { target: { value: "c-radarr:4" } });
    expect(onChange).toHaveBeenCalledWith({ connectionId: "c-radarr", profileId: 4 });
  });

  it("hides when requesters may not choose", () => {
    const { container } = render(
      <RequestQualityPicker
        options={{ enabled: false, servers: twoProfiles.servers }}
        value={null}
        onChange={vi.fn()}
      />,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
