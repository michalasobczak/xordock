import Foundation

// Tutorial (S010): introduction content, guided steps and a pure state
// machine. The tutorial performs nothing itself; it only observes the vault state.

public struct IntroPage: Equatable, Sendable, Identifiable {
    public var symbol: String
    public var title: String
    public var text: String
    public var id: String { title }
}

public enum Intro {
    public static let pages: [IntroPage] = [
        IntroPage(symbol: "laptopcomputer.trianglebadge.exclamationmark",
                  title: "Problem: everything in one place",
                  text: "Your most important files sit on a single computer. Whoever gets the Mac and access to the account also gets the files. Theft, a repair, a forgotten account: one event is enough."),
        IntroPage(symbol: "square.split.2x1",
                  title: "Idea: two halves",
                  text: "xordock splits every file into two halves, A and B. Each half on its own looks like random noise and reveals nothing: neither the contents nor the file names. Only both together make the file."),
        IntroPage(symbol: "sdcard",
                  title: "Mac holds A, card holds B",
                  text: "A stays on the Mac's disk, B goes to an SD card or USB drive. When you remove the card, there is nothing to read on the Mac. If you lose the card, on its own it reveals nothing. With the card plugged in, you see a regular folder in Finder."),
        IntroPage(symbol: "externaldrive.badge.icloud",
                  title: "Copies: spare card and iCloud",
                  text: "Each half is essential, so both have a copy: B on a second (spare) card, A in iCloud Drive. This way you can lose one thing at a time: the Mac, the working card or the spare card."),
        IntroPage(symbol: "checklist",
                  title: "Five rules",
                  text: "1. Keep the cards apart.\n2. Never put a card in iCloud or on a Time Machine disk.\n3. Unmount before you remove the card.\n4. Enable 2FA and Advanced Data Protection for iCloud.\n5. If you lose a card, restore the vault with new halves."),
        IntroPage(symbol: "hand.raised",
                  title: "Honestly: the limits of protection",
                  text: "The protection is practical, not absolute. While the vault is open, the files are available on the Mac as usual, and programs can make copies of them. So turn on FileVault and close the vault when you're not using it."),
    ]
}

/// UI element next to which the tip appears.
public enum TourAnchor: String, Sendable {
    case newVault, status, tiles, mount, unmount, backup, recovery, none
}

public enum TourStep: Int, CaseIterable, Sendable {
    case createVault, status, tiles, mount, useFinder, unmount, backup, recovery, menuBar

    public var anchor: TourAnchor {
        switch self {
        case .createVault: return .newVault
        case .status, .useFinder: return .status
        case .tiles: return .tiles
        case .mount: return .mount
        case .unmount: return .unmount
        case .backup: return .backup
        case .recovery: return .recovery
        case .menuBar: return .none
        }
    }

    /// The step completes by itself when the user performs the action.
    public var automatic: Bool {
        switch self {
        case .createVault, .mount, .unmount, .backup: return true
        default: return false
        }
    }

    public var title: String {
        switch self {
        case .createVault: return "Create your first vault"
        case .status: return "Vault status"
        case .tiles: return "Where the halves live"
        case .mount: return "Open the vault"
        case .useFinder: return "Work as in a regular folder"
        case .unmount: return "Close the vault"
        case .backup: return "Back up now"
        case .recovery: return "Recovery"
        case .menuBar: return "The lock in the menu bar"
        }
    }

    public var text: String {
        switch self {
        case .createVault:
            return "Connect a card or USB drive and click \"New vault…\". Enter a name and choose the drive. The tutorial continues once the vault is created."
        case .status:
            return "Here you always see the one most important thing and a button that takes care of it. Green means: all good."
        case .tiles:
            return "Four places: half A on the Mac, B on the working card, a copy of B on the spare card and a copy of A in iCloud. ⚠︎ shows what's missing."
        case .mount:
            return "Click \"Open vault\". The vault appears in Finder as a regular disk. The working card must be connected."
        case .useFinder:
            return "Drop a few files into the xordock window in Finder. Every write is split right away into half A (Mac) and B (card). Then click Next."
        case .unmount:
            return "Click \"Unmount\" before you remove the card. Otherwise the latest changes may be lost."
        case .backup:
            return "Connect a second drive (the spare card) and click \"Back up now\". This creates a copy of B on the card and a copy of A in iCloud."
        case .recovery:
            return "If something gets lost, recover your data here. Check your backup now and then: Recovery… → Verify."
        case .menuBar:
            return "The lock in the menu bar shows the state of your vaults and opens them without this window. Change the tutorial and the launch prompt in Settings (⌘,)."
        }
    }
}

/// Tutorial state machine (S010 T-04). Pure: it only observes state.
public struct Tour: Equatable, Sendable {
    public private(set) var step: TourStep?
    /// Backup point at the moment of entering the "back up now" step.
    public private(set) var baselinePoint: String?

    public init() {}

    public var active: Bool { step != nil }
    public var number: Int { (step?.rawValue ?? 0) + 1 }
    public static let count = TourStep.allCases.count

    public mutating func start(_ vault: VaultInfo?) {
        step = vault == nil ? .createVault : .status
        baselinePoint = nil
    }

    public mutating func finish() {
        step = nil
        baselinePoint = nil
    }

    public mutating func next(_ vault: VaultInfo?) {
        guard let s = step else { return }
        if s == .menuBar { finish(); return }
        enter(TourStep(rawValue: s.rawValue + 1)!, vault)
    }

    public mutating func back(_ vault: VaultInfo?) {
        guard let s = step, s.rawValue > 0 else { return }
        var prev = TourStep(rawValue: s.rawValue - 1)!
        if prev == .createVault && vault != nil { prev = .status }
        if prev != s { enter(prev, vault) }
    }

    private mutating func enter(_ s: TourStep, _ vault: VaultInfo?) {
        step = s
        if s == .backup { baselinePoint = vault?.lastBackup?.point }
    }

    /// Automatic transition once the step's action has been performed. Returns true
    /// if the step changed.
    @discardableResult
    public mutating func update(_ vault: VaultInfo?) -> Bool {
        guard let s = step, s.automatic else { return false }
        let done: Bool
        switch s {
        case .createVault: done = vault != nil
        case .mount: done = vault?.mounted == true
        case .unmount: done = vault?.mounted == false
        case .backup:
            if let p = vault?.lastBackup?.point { done = p != baselinePoint } else { done = false }
        default: done = false
        }
        if done { next(vault) }
        return done
    }
}
