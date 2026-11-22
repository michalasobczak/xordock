import Foundation

// Samouczek (S010): treść wprowadzenia, kroki prowadzenia i czysta maszyna
// stanów. Samouczek niczego nie wykonuje sam; tylko obserwuje stan sejfu.

public struct IntroPage: Equatable, Sendable, Identifiable {
    public var symbol: String
    public var title: String
    public var text: String
    public var id: String { title }
}

public enum Intro {
    public static let pages: [IntroPage] = [
        IntroPage(symbol: "laptopcomputer.trianglebadge.exclamationmark",
                  title: "Problem: wszystko w jednym miejscu",
                  text: "Twoje najważniejsze pliki leżą na jednym komputerze. Kto zdobędzie Maca i dostęp do konta, ma też pliki. Kradzież, serwis, zapomniane konto: wystarczy jedno zdarzenie."),
        IntroPage(symbol: "square.split.2x1",
                  title: "Pomysł: dwie połówki",
                  text: "xordock dzieli każdy plik na dwie połówki, A i B. Każda połówka osobno wygląda jak losowy szum i nic nie zdradza: ani treści, ani nazw plików. Dopiero obie razem dają plik."),
        IntroPage(symbol: "sdcard",
                  title: "A na Macu, B na karcie",
                  text: "A zostaje na dysku Maca, B trafia na kartę SD lub pendrive. Gdy wyjmiesz kartę, na Macu nie ma czego czytać. Gdy zgubisz kartę, sama nic nie zdradza. Z kartą w porcie widzisz zwykły folder w Finderze."),
        IntroPage(symbol: "externaldrive.badge.icloud",
                  title: "Kopie: zapasowa karta i iCloud",
                  text: "Każda połówka jest niezbędna, więc obie mają kopię: B na drugiej karcie (zapasowej), A w iCloud Drive. Dzięki temu możesz stracić jedną rzecz naraz: Maca, kartę roboczą albo zapasową."),
        IntroPage(symbol: "checklist",
                  title: "Pięć zasad",
                  text: "1. Nie trzymaj kart razem.\n2. Karta nigdy w iCloud ani na dysku Time Machine.\n3. Odmontuj, zanim wyjmiesz kartę.\n4. Włącz 2FA i Advanced Data Protection dla iCloud.\n5. Po zgubieniu karty odtwórz sejf z nowymi połówkami."),
        IntroPage(symbol: "hand.raised",
                  title: "Uczciwie: granice ochrony",
                  text: "Ochrona jest praktyczna, nie absolutna. Gdy sejf jest otwarty, pliki są dostępne na Macu jak zwykle, a programy mogą robić ich kopie. Dlatego włącz FileVault i zamykaj sejf, gdy go nie używasz."),
    ]
}

/// Element interfejsu, przy którym pojawia się dymek.
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

    /// Krok kończy się sam, gdy użytkownik wykona akcję.
    public var automatic: Bool {
        switch self {
        case .createVault, .mount, .unmount, .backup: return true
        default: return false
        }
    }

    public var title: String {
        switch self {
        case .createVault: return "Załóż pierwszy sejf"
        case .status: return "Status sejfu"
        case .tiles: return "Gdzie leżą połówki"
        case .mount: return "Otwórz sejf"
        case .useFinder: return "Pracuj jak w zwykłym folderze"
        case .unmount: return "Zamknij sejf"
        case .backup: return "Zrób backup"
        case .recovery: return "Odzyskiwanie"
        case .menuBar: return "Kłódka w pasku menu"
        }
    }

    public var text: String {
        switch self {
        case .createVault:
            return "Podłącz kartę lub pendrive i kliknij „Nowy sejf…”. Wpisz nazwę i wybierz nośnik. Samouczek przejdzie dalej, gdy sejf powstanie."
        case .status:
            return "Tu zawsze widzisz jedną najważniejszą rzecz i przycisk, który ją załatwia. Zielony znaczy: wszystko w porządku."
        case .tiles:
            return "Cztery miejsca: połówka A na Macu, B na karcie roboczej, kopia B na karcie zapasowej i kopia A w iCloud. ⚠︎ pokazuje, czego brakuje."
        case .mount:
            return "Kliknij „Otwórz sejf”. Sejf pojawi się w Finderze jako zwykły dysk. Karta robocza musi być podłączona."
        case .useFinder:
            return "Wrzuć kilka plików do okna xordock w Finderze. Każdy zapis od razu dzieli się na połówkę A (Mac) i B (karta). Potem kliknij Dalej."
        case .unmount:
            return "Kliknij „Odmontuj”, zanim wyjmiesz kartę. Bez tego ostatnie zmiany mogą przepaść."
        case .backup:
            return "Podłącz drugi nośnik (kartę zapasową) i kliknij „Zrób backup”. Powstanie kopia B na karcie i kopia A w iCloud."
        case .recovery:
            return "Gdy coś zginie, tu odzyskasz dane. Raz na jakiś czas sprawdź backup: Odzyskiwanie… → Weryfikuj."
        case .menuBar:
            return "Kłódka w pasku menu pokazuje stan sejfów i otwiera je bez tego okna. Samouczek i pytanie przy starcie zmienisz w Ustawieniach (⌘,)."
        }
    }
}

/// Maszyna stanów samouczka (S010 T-04). Czysta: tylko obserwuje stan.
public struct Tour: Equatable, Sendable {
    public private(set) var step: TourStep?
    /// Punkt backupu w chwili wejścia w krok „zrób backup”.
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

    /// Automatyczne przejście, gdy akcja kroku została wykonana. Zwraca true,
    /// jeśli krok się zmienił.
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
