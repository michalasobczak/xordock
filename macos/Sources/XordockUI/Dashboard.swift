import SwiftUI
import XordockCore

// Vault dashboard (S009): status with a single recommendation, drive tiles, action
// row, collapsed details. A view without decision logic (rules in Health.swift).

public extension Level {
    var color: Color {
        switch self {
        case .ok: return .green
        case .warning: return .orange
        case .error: return .red
        case .neutral: return .secondary
        }
    }
    var symbol: String {
        switch self {
        case .ok: return "checkmark.circle.fill"
        case .warning: return "exclamationmark.triangle.fill"
        case .error: return "xmark.octagon.fill"
        case .neutral: return "minus.circle"
        }
    }
}

/// Primary button: filled with the accent color (pure SwiftUI, renderable).
public struct PrimaryButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) private var enabled
    public init() {}
    public func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.body.weight(.semibold))
            .padding(.horizontal, 18).padding(.vertical, 9)
            .foregroundStyle(.white)
            .background(Color.accentColor.opacity(enabled ? (configuration.isPressed ? 0.75 : 1) : 0.4),
                        in: RoundedRectangle(cornerRadius: 9))
    }
}

/// Secondary button: outline, no fill.
public struct SecondaryButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) private var enabled
    public init() {}
    public func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .padding(.horizontal, 12).padding(.vertical, 7)
            .foregroundStyle(enabled ? Color.primary : Color.secondary.opacity(0.6))
            .background(Color.primary.opacity(configuration.isPressed ? 0.12 : 0.05), in: RoundedRectangle(cornerRadius: 8))
            .overlay(RoundedRectangle(cornerRadius: 8).strokeBorder(Color.primary.opacity(0.15)))
    }
}

public struct DashboardActions {
    public var primary: (PrimaryAction) -> Void = { _ in }
    public var mount: () -> Void = {}
    public var unmount: () -> Void = {}
    public var backup: () -> Void = {}
    public var recovery: () -> Void = {}
    public var fsck: () -> Void = {}
    public var showLog: () -> Void = {}
    public init() {}
}

public struct Dashboard: View {
    let vault: VaultInfo
    let icloud: Bool
    let masks: String
    let busy: Bool
    let actions: DashboardActions
    @State private var showDetails = false

    public init(vault: VaultInfo, icloud: Bool, masks: String, busy: Bool, actions: DashboardActions, showDetails: Bool = false) {
        self.vault = vault
        self.icloud = icloud
        self.masks = masks
        self.busy = busy
        self.actions = actions
        _showDetails = State(initialValue: showDetails)
    }

    public var body: some View {
        ScrollView { content }
    }

    /// Dashboard content without scrolling (renderable by ImageRenderer).
    public var content: some View {
        let health = VaultHealth.evaluate(vault, icloud: icloud)
        let tiles = vault.tiles(icloud: icloud)
        return VStack(alignment: .leading, spacing: 20) {
            Text(vault.name).font(.largeTitle.weight(.semibold))
            StatusCard(health: health, busy: busy) { actions.primary(health.action) }
                .tourAnchor(.status)
            ViewThatFits(in: .horizontal) {
                HStack(spacing: 12) { ForEach(tiles) { TileView(tile: $0).frame(minWidth: 140) } }
                VStack(spacing: 12) {
                    HStack(spacing: 12) { ForEach(tiles.prefix(2)) { TileView(tile: $0) } }
                    HStack(spacing: 12) { ForEach(tiles.suffix(2)) { TileView(tile: $0) } }
                }
            }
            .tourAnchor(.tiles)
            actionRow(primary: health.action)
            details
        }
        .padding(24)
        .frame(maxWidth: 820, alignment: .leading)
    }

    /// Secondary actions; the one that is the primary button is not repeated.
    private func actionRow(primary: PrimaryAction) -> some View {
        HStack(spacing: 10) {
            if vault.mounted {
                Button("Unmount", systemImage: "eject", action: actions.unmount)
                    .tourAnchor(.unmount)
            } else if primary != .mount {
                Button("Open vault", systemImage: "lock.open", action: actions.mount)
                    .disabled(vault.card == nil || vault.error != nil)
                    .tourAnchor(.mount)
            }
            if primary != .backup {
                Button("Backup…", systemImage: "arrow.triangle.2.circlepath", action: actions.backup)
                    .disabled(vault.card == nil)
                    .tourAnchor(.backup)
            }
            Button("Recovery…", systemImage: "lifepreserver", action: actions.recovery)
                .tourAnchor(.recovery)
            Spacer()
            Menu {
                Button("Check consistency", action: actions.fsck).disabled(vault.card == nil || vault.mounted)
                Button("Show mount log", action: actions.showLog)
            } label: {
                Label("More", systemImage: "ellipsis.circle")
            }
            .menuStyle(.button)
            .buttonStyle(SecondaryButtonStyle())
            .menuIndicator(.hidden)
            .fixedSize()
        }
        .buttonStyle(SecondaryButtonStyle())
        .disabled(busy)
    }

    private var details: some View {
        DisclosureGroup("Technical details", isExpanded: $showDetails) {
            Grid(alignment: .leading, horizontalSpacing: 12, verticalSpacing: 6) {
                row("Share A", vault.a)
                row("Working card", vault.card ?? "not connected")
                if let c = vault.aCopy { row("Copy of A", c.path) }
                ForEach(vault.spares, id: \.self) { row("Spare card", $0.root) }
                row("Commit / snapshots", "\(vault.seq) / \(vault.snapshots)")
                row("Masks", masks)
            }
            .font(.callout)
            .textSelection(.enabled)
            .padding(.top, 8)
        }
        .foregroundStyle(.secondary)
    }

    private func row(_ k: String, _ v: String) -> some View {
        GridRow {
            Text(k).foregroundStyle(.secondary)
            Text(v).foregroundStyle(.primary).fixedSize(horizontal: false, vertical: true)
        }
    }
}

struct StatusCard: View {
    let health: VaultHealth
    let busy: Bool
    let onAction: () -> Void

    var body: some View {
        HStack(alignment: .center, spacing: 16) {
            Image(systemName: health.level.symbol)
                .font(.system(size: 40))
                .foregroundStyle(health.level.color)
            VStack(alignment: .leading, spacing: 4) {
                Text(health.title).font(.title2.weight(.semibold))
                Text(health.detail).foregroundStyle(.secondary)
            }
            Spacer(minLength: 12)
            if let t = health.actionTitle {
                Button(t, action: onAction)
                    .buttonStyle(PrimaryButtonStyle())
                    .keyboardShortcut(.defaultAction)
                    .disabled(busy)
                    .tourAnchor(health.action == .mount ? .mount : (health.action == .backup ? .backup : .none))
            }
        }
        .padding(20)
        .background(health.level.color.opacity(0.12), in: RoundedRectangle(cornerRadius: 14))
        .overlay(RoundedRectangle(cornerRadius: 14).strokeBorder(health.level.color.opacity(0.35)))
    }
}

struct TileView: View {
    let tile: Tile

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Image(systemName: tile.symbol).font(.title2).foregroundStyle(.secondary)
                Spacer()
                Image(systemName: tile.level.symbol).foregroundStyle(tile.level.color)
            }
            .frame(height: 26)
            Text(tile.title).font(.headline)
            Text(tile.role).font(.caption).foregroundStyle(.secondary)
            Text(tile.status).font(.callout.weight(.medium)).foregroundStyle(tile.level == .warning ? Color.orange : .primary)
            Text(tile.note ?? " ").font(.caption).foregroundStyle(.secondary).lineLimit(1)
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.background.secondary, in: RoundedRectangle(cornerRadius: 12))
        .overlay(RoundedRectangle(cornerRadius: 12).strokeBorder(.separator))
    }
}

/// Status dot in the vault list (S009 P-05).
public struct HealthDot: View {
    let level: Level
    public init(level: Level) { self.level = level }
    public var body: some View {
        Circle().fill(level.color).frame(width: 9, height: 9)
    }
}
